package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"sort"
	"strings"

	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"

	"github.com/RushanM/Minecraft-Mods-Russian-Translation/tools/common"
)

// ProofreadDates содержит даты последней проверки для модов
type ProofreadDates map[string]string

// ModInfo содержит информацию о моде
type ModInfo struct {
	Name         string
	GameVer      string
	Proofread    string
	ModrinthID   string
	CurseforgeID string
	FallbackURL  string
	Status       string
	URL          string
	Entry        string
}

func GenerateModList() {
	// Загрузка предыдущих данных
	previousProofreadDates := make(ProofreadDates)
	prevDatesFile := "previous_proofread_dates.json"

	if _, err := os.Stat(prevDatesFile); err == nil {
		data, err := ioutil.ReadFile(prevDatesFile)
		if err != nil {
			fmt.Printf("Ошибка при чтении %s: %v\n", prevDatesFile, err)
		} else {
			err = json.Unmarshal(data, &previousProofreadDates)
			if err != nil {
				fmt.Printf("Ошибка при разборе JSON в %s: %v\n", prevDatesFile, err)
			}
		}
	} else {
		fmt.Println("Файл previous_proofread_dates.json не найден. Предполагается первый запуск.")
	}

	// Подключение к Google Sheets API
	ctx := context.Background()
	serviceAccountKey := os.Getenv("GOOGLE_SERVICE_ACCOUNT_KEY")
	if serviceAccountKey == "" {
		fmt.Println("Не установлена переменная окружения GOOGLE_SERVICE_ACCOUNT_KEY")
		os.Exit(1)
	}

	config, err := google.JWTConfigFromJSON([]byte(serviceAccountKey), sheets.SpreadsheetsReadonlyScope)
	if err != nil {
		fmt.Printf("Не удалось создать конфигурацию JWT: %v\n", err)
		os.Exit(1)
	}

	client := config.Client(ctx)
	srv, err := sheets.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		fmt.Printf("Не удалось создать сервис Sheets: %v\n", err)
		os.Exit(1)
	}

	// Получение данных из таблицы
	sheetID := "1kGGT2GGdG_Ed13gQfn01tDq2MZlVOC9AoiD1s3SDlZE"
	readRange := "Sheet1!A:Z"
	resp, err := srv.Spreadsheets.Values.Get(sheetID, readRange).Do()
	if err != nil {
		fmt.Printf("Не удалось получить данные из таблицы: %v\n", err)
		os.Exit(1)
	}

	if len(resp.Values) == 0 {
		fmt.Println("Данные не найдены.")
		os.Exit(1)
	}

	// Получение заголовков и индексов колонок
	headers := make(map[string]int)
	for i, header := range resp.Values[0] {
		headers[header.(string)] = i
	}

	// Обработка данных
	modsByVersion := make(map[string][]ModInfo)
	currentProofreadDates := make(ProofreadDates)

	for i := 1; i < len(resp.Values); i++ {
		row := resp.Values[i]
		if len(row) <= headers["proofread"] {
			continue
		}

		proofread := common.GetValueAsString(row, headers["proofread"])
		if proofread == "" || strings.ToUpper(proofread) == "FALSE" {
			continue
		}

		modName := common.GetValueAsString(row, headers["name"])
		gameVer := common.GetValueAsString(row, headers["gameVer"])
		modrinthID := common.GetValueAsString(row, headers["modrinthId"])
		curseforgeID := common.GetValueAsString(row, headers["curseforgeId"])
		fallbackURL := common.GetValueAsString(row, headers["fallbackUrl"])

		// Сохранение текущей даты
		currentProofreadDates[modName] = proofread

		// Определение статуса перевода
		status := "unchanged"
		prevProofreadDate, exists := previousProofreadDates[modName]
		if !exists {
			status = "new" // Новый мод
		} else if prevProofreadDate != proofread {
			status = "updated" // Обновлённый мод
		}

		// Получение ссылки на мод
		modURL := getModURL(modrinthID, curseforgeID, fallbackURL)

		// Формирование строки мода
		dateStr := fmt.Sprintf("<code>%s</code>", proofread)
		var modLink string
		if modURL != "" {
			modLink = fmt.Sprintf("<a href=\"%s\">%s</a>", modURL, modName)
		} else {
			modLink = modName
		}

		// Добавление эмодзи и форматирования
		var modEntry string
		if status == "new" {
			emoji := "➕"
			modEntry = fmt.Sprintf("<li><b>%s %s %s</b></li>", emoji, modLink, dateStr)
		} else if status == "updated" {
			emoji := "✏️"
			modEntry = fmt.Sprintf("<li><b>%s %s %s</b></li>", emoji, modLink, dateStr)
		} else {
			modEntry = fmt.Sprintf("<li>%s %s</li>", modLink, dateStr)
		}

		// Добавляем мод в соответствующую версию игры
		mod := ModInfo{
			Name:         modName,
			GameVer:      gameVer,
			Proofread:    proofread,
			ModrinthID:   modrinthID,
			CurseforgeID: curseforgeID,
			FallbackURL:  fallbackURL,
			Status:       status,
			URL:          modURL,
			Entry:        modEntry,
		}

		modsByVersion[gameVer] = append(modsByVersion[gameVer], mod)
	}

	// Генерация тела выпуска
	releaseBody := generateReleaseBody(modsByVersion)

	// Сохранение текущих данных
	currentData, err := json.MarshalIndent(currentProofreadDates, "", "    ")
	if err != nil {
		fmt.Printf("Не удалось сериализовать данные: %v\n", err)
		os.Exit(1)
	}

	err = ioutil.WriteFile("current_proofread_dates.json", currentData, 0644)
	if err != nil {
		fmt.Printf("Не удалось записать данные в файл: %v\n", err)
		os.Exit(1)
	}

	// Установка выходного значения для release_body
	githubOutput := os.Getenv("GITHUB_OUTPUT")
	if githubOutput != "" {
		file, err := os.OpenFile(githubOutput, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			fmt.Printf("Не удалось открыть GITHUB_OUTPUT: %v\n", err)
			os.Exit(1)
		}
		defer file.Close()

		_, err = file.WriteString("release_body<<EOF\n")
		if err != nil {
			fmt.Printf("Не удалось записать в GITHUB_OUTPUT: %v\n", err)
			os.Exit(1)
		}

		_, err = file.WriteString(releaseBody)
		if err != nil {
			fmt.Printf("Не удалось записать release_body в GITHUB_OUTPUT: %v\n", err)
			os.Exit(1)
		}

		_, err = file.WriteString("\nEOF\n")
		if err != nil {
			fmt.Printf("Не удалось закрыть EOF в GITHUB_OUTPUT: %v\n", err)
			os.Exit(1)
		}
	} else {
		fmt.Println(releaseBody)
	}
}

// getModURL получает URL мода из API Modrinth или CurseForge
func getModURL(modrinthID, curseforgeID, fallbackURL string) string {
	if modrinthID != "" && strings.ToUpper(modrinthID) != "FALSE" {
		// Modrinth
		resp, err := http.Get(fmt.Sprintf("https://api.modrinth.com/v2/project/%s", modrinthID))
		if err == nil && resp.StatusCode == http.StatusOK {
			var modData map[string]interface{}
			if err := json.NewDecoder(resp.Body).Decode(&modData); err == nil {
				if url, ok := modData["url"].(string); ok && url != "" {
					return url
				}
			}
			resp.Body.Close()
		}
		return fmt.Sprintf("https://modrinth.com/mod/%s", modrinthID)
	} else if curseforgeID != "" && strings.ToUpper(curseforgeID) != "FALSE" {
		// CurseForge
		apiKey := os.Getenv("CF_API_KEY")
		if apiKey != "" {
			client := &http.Client{}
			req, err := http.NewRequest("GET", fmt.Sprintf("https://api.curseforge.com/v1/mods/%s", curseforgeID), nil)
			if err == nil {
				req.Header.Add("Accept", "application/json")
				req.Header.Add("x-api-key", apiKey)
				resp, err := client.Do(req)
				if err == nil && resp.StatusCode == http.StatusOK {
					var cfResp struct {
						Data struct {
							Links struct {
								WebsiteURL string `json:"websiteUrl"`
							} `json:"links"`
						} `json:"data"`
					}
					if err := json.NewDecoder(resp.Body).Decode(&cfResp); err == nil {
						if cfResp.Data.Links.WebsiteURL != "" {
							return cfResp.Data.Links.WebsiteURL
						}
					}
					resp.Body.Close()
				}
			}
		}
		return fmt.Sprintf("https://www.curseforge.com/minecraft/mc-mods/%s", curseforgeID)
	} else if fallbackURL != "" && strings.ToUpper(fallbackURL) != "FALSE" {
		return fallbackURL // Используем ссылку из fallbackUrl
	}
	return "" // Не удалось получить ссылку
}

// generateReleaseBody создаёт тело для выпуска в формате HTML/Markdown
func generateReleaseBody(modsByVersion map[string][]ModInfo) string {
	// Начало тела выпуска
	releaseBody := `Это бета-выпуск всех переводов проекта. В отличие от альфа-выпуска, качество переводов здесь значительно выше, поскольку включены только те переводы, чьё качество достигло достаточно высокого уровня. Однако из-за этого охваченный спектр модов, сборок модов и наборов шейдеров значительно уже.

<details>
    <summary>
        <h3>🔠 Переведённые моды этого выпуска</h3>
    </summary>
    <br>
    <b>Условные обозначения</b>
    <br><br>
    <ul>
        <li>➕ — новый перевод</li>
        <li>✏️ — изменения в переводе</li>
        <li><code>ДД.ММ.ГГГГ</code> — дата последнего изменения</li>
    </ul>
    <br>
`

	// Сортировка версий игры
	gameVersions := make([]string, 0, len(modsByVersion))
	for gameVer := range modsByVersion {
		gameVersions = append(gameVersions, gameVer)
	}
	sort.Strings(gameVersions)

	// Для каждой версии игры создаём спойлер
	for _, gameVer := range gameVersions {
		mods := modsByVersion[gameVer]

		// Сортировка модов внутри версии по дате (новые выше)
		sort.Slice(mods, func(i, j int) bool {
			return mods[i].Proofread > mods[j].Proofread
		})

		// Получаем последнюю дату для версии
		var latestDate string
		if len(mods) > 0 {
			latestDate = mods[0].Proofread
		}

		// Определяем, есть ли новые или обновлённые моды
		versionStatus := ""
		for _, mod := range mods {
			if mod.Status == "new" || mod.Status == "updated" {
				versionStatus = "✏️"
				break
			}
		}

		// Формируем заголовок спойлера для версии
		versionHeader := fmt.Sprintf("<summary><b>%s", gameVer)
		if versionStatus != "" {
			versionHeader += fmt.Sprintf(" %s", versionStatus)
		}
		versionHeader += fmt.Sprintf(" <code>%s</code></b></summary>", latestDate)
		releaseBody += fmt.Sprintf("    <details>\n        %s\n        <ul>\n", versionHeader)

		// Добавляем моды
		for _, mod := range mods {
			releaseBody += fmt.Sprintf("            %s\n", mod.Entry)
		}

		releaseBody += "        </ul>\n    </details>\n"
	}

	releaseBody += "</details>\n\nЭтот выпуск является кандидатом на релиз. Если вы заметили какие-либо ошибки в этом выпуске, пожалуйста, сообщите об этом в разделе issues или отправьте сообщение [Дефлекте](https://github.com/RushanM)!"

	return releaseBody
}
