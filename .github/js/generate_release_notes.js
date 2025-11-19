// Сценарий для автоматической генерации списка изменений при выпуске новых версий

const { google } = require('googleapis');
const core = require('@actions/core');
const github = require('@actions/github');
const { execSync } = require('child_process');

(async () => {
    try {
        const token = process.env.GITHUB_TOKEN;
        const octokit = github.getOctokit(token);
        const previousTag = process.env.previous_alpha_tag;
        let commitRange = '';
        if (previousTag) {
            commitRange = `${previousTag}..HEAD`;
        } else {
            commitRange = 'HEAD';
        }

        // Получение списка изменённых файлов
        let files = [];
        try {
            const output = execSync(`git diff --name-only ${commitRange}`).toString();
            files = output.trim().split('\n');
        } catch (error) {
            core.setFailed(`Failed to get changed files: ${error.message}`);
            return;
        }

        // Фильтрация файлов переводов
        const translationFiles = files.filter(file => file.includes('assets/') && file.includes('/lang/'));

        // Извлечение изменённых переводов модов и версий
        const modChanges = {};
        translationFiles.forEach(file => {
            const parts = file.split('/');
            const version = parts[1];
            const modIdIndex = parts.indexOf('assets') + 1;
            const modId = parts[modIdIndex];
            const langFile = parts.slice(modIdIndex + 1).join('/');

            if (!modChanges[modId]) {
                modChanges[modId] = new Set();
            }
            modChanges[modId].add(version);
        });

        // Подключение к БД
        const sheets = google.sheets('v4');

        const auth = new google.auth.GoogleAuth({
            credentials: JSON.parse(process.env.GOOGLE_SERVICE_ACCOUNT_KEY),
            scopes: ['https://www.googleapis.com/auth/spreadsheets.readonly'],
        });

        const docId = '1kGGT2GGdG_Ed13gQfn01tDq2MZlVOC9AoiD1s3SDlZE';

        let modData = {};
        let modOrder = []; // Массив для хранения порядка модов

        try {
            const response = await sheets.spreadsheets.values.get({
                auth,
                spreadsheetId: docId,
                range: 'db!A:I', // Лист «db»
            });

            const rows = response.data.values;
            if (rows.length) {
                // Заголовки: id, name, modrinthUrl, cfUrl, fallbackUrl
                const headers = rows[0];
                for (let i = 1; i < rows.length; i++) {
                    const row = rows[i];
                    const mod = {};
                    for (let j = 0; j < headers.length; j++) {
                        mod[headers[j]] = row[j];
                    }
                    const modId = mod['id'];
                    modData[modId] = mod;

                    // Сохранение порядка модов в массиве modOrder
                    modOrder.push(modId);
                }
            } else {
                core.setFailed('В гугл-таблице нет данных');
                return;
            }
        } catch (error) {
            core.setFailed(`Ошибка доступа к гугл-таблице: ${error.message}`);
            return;
        }

        // Построение списка изменений

        // Заголовок
        const alphaNumber = parseInt(process.env.next_alpha, 10);
        let releaseNotes = `Это ${alphaNumber}-я альфа-версия всех переводов проекта.\n\n`;
        releaseNotes += `Про то, как выходят ранние версии проекта, можете прочитать [здесь](https://github.com/RushanM/Minecraft-Mods-Russian-Translation/blob/master/Памятки/Именование%20выпусков.md).\n\n`;

        // Изменения
        const changes = {};

        // Сборка изменений
        translationFiles.forEach(file => {
            const parts = file.split('/');
            const version = parts[1];
            const modIdIndex = parts.indexOf('assets') + 1;
            const modId = parts[modIdIndex];
            const langFile = parts.slice(modIdIndex + 1).join('/');

            const mod = modData[modId];
            if (mod) {
                let modName = mod['name'];
                let modUrl = mod['modrinthUrl'];
                if (!modUrl || modUrl === 'FALSE') {
                    modUrl = mod['cfUrl'];
                }
                if (!modUrl || modUrl === 'FALSE') {
                    modUrl = mod['fallbackUrl'];
                }

                // Определение языка
                let lang = '';
                if (file.includes('ru_ru')) {
                    lang = 'русский';
                } else if (file.includes('tt_ru')) {
                    lang = 'татарский';
                } else {
                    lang = 'неизвестный язык';
                }

                // Инициализация массива для языка, если он ещё не создан
                if (!changes[lang]) {
                    changes[lang] = {};
                }

                // Инициализация объекта для мода, если он ещё не создан
                if (!changes[lang][modId]) {
                    changes[lang][modId] = {
                        modName: modName,
                        modUrl: modUrl,
                        versions: new Set(),
                    };
                }

                // Добавление версии в список версий мода
                changes[lang][modId].versions.add(version);
            }
        });

        // Формирование releaseNotes
        if (Object.keys(changes).length === 0) {
            releaseNotes += 'В этой альфе нет изменений.';
        } else {
            releaseNotes += '**Изменения**\n\n';

            // Проход по языкам
            for (const lang in changes) {
                releaseNotes += `* Изменён ${lang} перевод:\n`;

                const mods = changes[lang];

                // Преобразование объекта модов в массив и сортировка по порядку из modOrder
                const modEntries = Object.entries(mods).sort((a, b) => {
                    const indexA = modOrder.indexOf(a[0]);
                    const indexB = modOrder.indexOf(b[0]);
                    return indexA - indexB;
                });

                const modLines = [];

                // Формирование строки для каждого мода
                for (let [modId, modChange] of modEntries) {
                    const versions = Array.from(modChange.versions).sort();
                    modLines.push(`  * [${modChange.modName}](${modChange.modUrl}) на Minecraft ${versions.join(', ')}`);
                }

                // Форматирование списка модов с правильной пунктуацией
                for (let i = 0; i < modLines.length; i++) {
                    if (i === modLines.length - 1) {
                        releaseNotes += `${modLines[i]}.`; // Последний элемент заканчивается точкой
                    } else {
                        releaseNotes += `${modLines[i]};\n`; // Остальные точкой с запятой и переносом строки
                    }
                }

                releaseNotes += '\n'; // Пустая строка после каждого языка
            }
        }

        core.setOutput('release_notes', releaseNotes);
    } catch (error) {
        core.setFailed(error.message);
    }
})();