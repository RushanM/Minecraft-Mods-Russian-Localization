package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"

	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"

	"github.com/RushanM/Minecraft-Mods-Russian-Translation/tools/common"
)

// RequestData содержит информацию о запросе на перевод мода
type RequestData struct {
	Name         string
	GameVer      string
	ModrinthID   string
	CurseforgeID string
}

// ModCount содержит название мода и количество запросов
type ModCount struct {
	Name  string
	Count int
}

// Constants
const (
	SPREADSHEET_ID = "1kGGT2GGdG_Ed13gQfn01tDq2MZlVOC9AoiD1s3SDlZE"
	REQUESTS_SHEET = "requests"
	NUM_TOP_MODS   = 4
)

func UpdateReadme() {
	// Аутентификация с помощью служебной учётной записи
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
	readRange := fmt.Sprintf("%s!A1:Z1000", REQUESTS_SHEET)
	resp, err := srv.Spreadsheets.Values.Get(SPREADSHEET_ID, readRange).Do()
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

	// Обработка данных запросов
	data := make([]RequestData, 0)
	for i := 1; i < len(resp.Values); i++ {
		row := resp.Values[i]
		if len(row) <= headers["name"] {
			continue
		}

		name := common.GetValueAsString(row, headers["name"])
		if name == "" {
			continue
		}

		gameVer := ""
		if headers["gameVer"] < len(row) {
			gameVer = common.GetValueAsString(row, headers["gameVer"])
		}

		modrinthID := ""
		if headers["modrinthId"] < len(row) {
			modrinthID = common.GetValueAsString(row, headers["modrinthId"])
		}

		curseforgeID := ""
		if headers["curseforgeId"] < len(row) {
			curseforgeID = common.GetValueAsString(row, headers["curseforgeId"])
		}

		data = append(data, RequestData{
			Name:         name,
			GameVer:      gameVer,
			ModrinthID:   modrinthID,
			CurseforgeID: curseforgeID,
		})
	}

	// Подсчёт запросов для каждого мода
	modCounts := countModRequests(data)

	// Получение топ-модов
	topMods := getTopMods(modCounts, NUM_TOP_MODS)

	// Генерация таблицы модов
	tableRows := generateModsTable(topMods, data)

	// Обновление README.md
	updateReadme(tableRows)
}

// countModRequests подсчитывает количество запросов для каждого мода
func countModRequests(data []RequestData) map[string]int {
	counts := make(map[string]int)
	for _, req := range data {
		counts[req.Name]++
	}
	return counts
}

// getTopMods возвращает указанное количество самых запрашиваемых модов
func getTopMods(counts map[string]int, limit int) []ModCount {
	// Сортировка по количеству запросов
	var modCounts []ModCount
	for name, count := range counts {
		modCounts = append(modCounts, ModCount{Name: name, Count: count})
	}

	sort.Slice(modCounts, func(i, j int) bool {
		return modCounts[i].Count > modCounts[j].Count
	})

	// Возвращаем топ-N модов или меньше, если модов меньше указанного лимита
	if len(modCounts) < limit {
		limit = len(modCounts)
	}
	return modCounts[:limit]
}

// getModInfo получает информацию о моде
func getModInfo(modName string, data []RequestData) RequestData {
	// Фильтруем записи для данного мода
	var modEntries []RequestData
	for _, entry := range data {
		if entry.Name == modName {
			modEntries = append(modEntries, entry)
		}
	}

	if len(modEntries) == 0 {
		return RequestData{Name: modName}
	}

	// Подсчёт версий игры
	gameVerCounts := make(map[string]int)
	for _, entry := range modEntries {
		if entry.GameVer != "" {
			gameVerCounts[entry.GameVer]++
		}
	}

	// Определение наиболее частой версии игры
	var mostCommonGameVer string
	var maxCount int
	for ver, count := range gameVerCounts {
		if count > maxCount {
			maxCount = count
			mostCommonGameVer = ver
		}
	}

	// Берём первую запись для получения modrinthId, curseforgeId
	result := modEntries[0]
	result.GameVer = mostCommonGameVer

	return result
}

// fetchModIconAndLink получает значок и ссылку на мод
func fetchModIconAndLink(mod RequestData) (string, string) {
	iconURL := ""
	modLink := ""

	if mod.ModrinthID != "" && strings.ToUpper(mod.ModrinthID) != "FALSE" {
		// Используем API Модринта
		resp, err := http.Get(fmt.Sprintf("https://api.modrinth.com/v2/project/%s", mod.ModrinthID))
		if err == nil && resp.StatusCode == http.StatusOK {
			var modData map[string]interface{}
			if err := json.NewDecoder(resp.Body).Decode(&modData); err == nil {
				if icon, ok := modData["icon_url"].(string); ok {
					iconURL = icon
				}
				if website, ok := modData["website_url"].(string); ok {
					modLink = website
				} else {
					modLink = fmt.Sprintf("https://modrinth.com/mod/%s", mod.ModrinthID)
				}
			}
			resp.Body.Close()
		} else {
			fmt.Printf("Не удалось получить данные %s с Модринта\n", mod.Name)
		}
	} else if mod.CurseforgeID != "" && strings.ToUpper(mod.CurseforgeID) != "FALSE" {
		// Используем API Кёрсфорджа
		fmt.Printf("Смотрим мод %s, идентификатор у него: %s\n", mod.Name, mod.CurseforgeID)
		apiToken := os.Getenv("CFCORE_API_TOKEN")
		if apiToken != "" {
			client := &http.Client{}
			req, err := http.NewRequest("GET", fmt.Sprintf("https://api.curseforge.com/v1/mods/%s", mod.CurseforgeID), nil)
			if err == nil {
				// Использовать правильный заголовок для аутентификации
				req.Header.Add("X-Api-Token", apiToken)
				resp, err := client.Do(req)
				if err == nil && resp.StatusCode == http.StatusOK {
					var cfResp map[string]interface{}
					if err := json.NewDecoder(resp.Body).Decode(&cfResp); err == nil {
						if data, ok := cfResp["data"].(map[string]interface{}); ok {
							if logo, ok := data["logo"].(map[string]interface{}); ok {
								if url, ok := logo["url"].(string); ok {
									iconURL = url
								}
							}
							if links, ok := data["links"].(map[string]interface{}); ok {
								if url, ok := links["websiteUrl"].(string); ok {
									modLink = url
								}
							}
						}
					}
					resp.Body.Close()
				} else {
					fmt.Printf("Не удалось получить данные %s с Кёрсфорджа\n", mod.Name)
					if resp != nil {
						fmt.Printf("Состояние HTTP: %d, ответ получен такой: %s\n", resp.StatusCode, resp.Status)
					}
				}
			}
		} else {
			fmt.Println("CFCORE_API_TOKEN не установлен.")
		}
	} else {
		fmt.Printf("У %s нет ни верного modrinthId ни верного curseforgeId\n", mod.Name)
	}

	return iconURL, modLink
}

// declineProsba склоняет слово "просьба" в зависимости от числа
func declineProsba(n int) string {
	absN := n
	if absN < 0 {
		absN = -absN
	}

	nMod10 := absN % 10
	nMod100 := absN % 100

	if nMod10 == 1 && nMod100 != 11 {
		return "просьба"
	} else if (nMod10 >= 2 && nMod10 <= 4) && (nMod100 < 12 || nMod100 > 14) {
		return "просьбы"
	} else {
		return "просьб"
	}
}

// generateModsTable генерирует таблицу модов
func generateModsTable(topMods []ModCount, data []RequestData) []string {
	var tableRows []string

	for _, modCount := range topMods {
		modInfo := getModInfo(modCount.Name, data)
		iconURL, modLink := fetchModIconAndLink(modInfo)

		// Проверяем, используется ли значок с Модринта
		var iconHTML string
		if iconURL != "" && strings.Contains(iconURL, "modrinth") {
			iconHTML = fmt.Sprintf("<img width=80 height=80 src=\"%s\">", iconURL)
		} else {
			// Используем запасной значок
			iconHTML = "<img width=80 height=80 src=\"/Ассеты/curseforge_mod_vector.svg\">"
		}

		var modLinkHTML string
		if modLink != "" {
			modLinkHTML = fmt.Sprintf("**[%s](%s)**", modInfo.Name, modLink)
		} else {
			modLinkHTML = fmt.Sprintf("**%s**", modInfo.Name)
		}

		prosbForm := declineProsba(modCount.Count)
		tableCell := fmt.Sprintf("<big>%s</big><br>%s<br>*%d %s*", modLinkHTML, modInfo.GameVer, modCount.Count, prosbForm)
		tableRows = append(tableRows, fmt.Sprintf("| %s | %s |", iconHTML, tableCell))
	}

	return tableRows
}

// updateReadme обновляет таблицу востребованных модов в README.md
func updateReadme(tableRows []string) {
	// Определяем список возможных расположений файла README.md
	possiblePaths := []string{
		"README.md",               // Корневая директория
		"../README.md",            // Уровень выше
		"../../README.md",         // Два уровня выше
		".github/README.md",       // В папке .github
		"../.github/README.md",    // Уровень выше в папке .github
		"../../.github/README.md", // Два уровня выше в папке .github
	}

	// Ищем файл README.md в возможных расположениях
	var readmePath string
	for _, path := range possiblePaths {
		if _, err := os.Stat(path); err == nil {
			readmePath = path
			fmt.Printf("Найден README.md по пути: %s\n", readmePath)
			break
		}
	}

	// Если файл не найден, выходим с ошибкой
	if readmePath == "" {
		fmt.Println("Не удалось найти README.md ни в одном из ожидаемых расположений")
		os.Exit(1)
	}

	// Читаем README.md
	content, err := ioutil.ReadFile(readmePath)
	if err != nil {
		fmt.Printf("Не удалось прочитать README.md по пути %s: %v\n", readmePath, err)
		os.Exit(1)
	}

	// Вывести содержимое файла для отладки
	fmt.Println("Содержимое файла README.md (первые 500 символов):")
	if len(content) > 500 {
		fmt.Println(string(content[:500]) + "...")
	} else {
		fmt.Println(string(content))
	}

	// Формируем новую таблицу
	tableHeader := "| Значок | Описание |\n| :-: | :-: |"
	table := tableHeader + "\n" + strings.Join(tableRows, "\n")

	// Сохраняем путь к README.md для последующего использования
	if err := ioutil.WriteFile("readme_path.txt", []byte(readmePath), 0644); err != nil {
		fmt.Printf("Не удалось сохранить путь к README.md: %v\n", err)
	} else {
		fmt.Printf("Путь к README.md сохранён в файле readme_path.txt: %s\n", readmePath)
	}

	// Регулярное выражение для поиска существующей таблицы
	re := regexp.MustCompile(`(?s)## Моды востребованные для перевода.*?<div align=center>.*?</div>`)
	strContent := string(content)

	// Заменяем существующую таблицу на новую
	newSection := fmt.Sprintf("## Моды востребованные для перевода\n\nЛюди больше всего просят перевести эти моды, но они до сих пор не переведены из-за размеров перевода. Если вы нашли в себе силы и имеете достаточно опыта, можете взяться за них.\n\n<div align=center>\n\n%s\n\n</div>", table)

	var newContent string
	if re.MatchString(strContent) {
		newContent = re.ReplaceAllString(strContent, newSection)
		fmt.Println("Раздел с таблицей востребованных модов найден и обновлён.")
	} else {
		fmt.Println("Раздел с таблицей востребованных модов не найден в README.md.")
		fmt.Println("Проверим содержимое на наличие ключевых слов:")

		if strings.Contains(strContent, "Моды востребованные для перевода") {
			fmt.Println("Заголовок 'Моды востребованные для перевода' найден.")
		} else {
			fmt.Println("Заголовок 'Моды востребованные для перевода' НЕ найден.")
		}

		if strings.Contains(strContent, "<div align=center>") {
			fmt.Println("Тег '<div align=center>' найден.")
		} else {
			fmt.Println("Тег '<div align=center>' НЕ найден.")
		}

		if strings.Contains(strContent, "</div>") {
			fmt.Println("Тег '</div>' найден.")
		} else {
			fmt.Println("Тег '</div>' НЕ найден.")
		}
		return
	}

	// Записываем обновлённый README.md
	err = ioutil.WriteFile(readmePath, []byte(newContent), 0644)
	if err != nil {
		fmt.Printf("Не удалось записать обновлённый README.md: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("README.md успешно обновлён.")
}
