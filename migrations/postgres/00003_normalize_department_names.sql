-- +goose Up
-- Приводит устаревшие названия отделений (эпоха Playwright-скрейпера, меню mnokol.tyuiu.ru)
-- к каноническим коротким кодам, которые сейчас отдаёт coworking.tyuiu.ru funct.php.
-- "Отделение МПН" распалось на два подразделения; разбиваем по коду группы.
UPDATE chats SET department = CASE
    WHEN department = 'Отделение АиЭС' THEN 'АиЭС'
    WHEN department = 'Отделение НГО' THEN 'НГО'
    WHEN department = 'Отделение СОНХ' THEN 'СОНХ'
    WHEN department = 'Отделение ПО' THEN 'Политехническое'
    WHEN department = 'Отделение МПН' AND "group" LIKE 'ПНГт%' THEN 'МПН Осипенко'
    WHEN department = 'Отделение МПН' AND (
        "group" LIKE 'МТОт%' OR "group" LIKE 'ТМт%' OR "group" LIKE 'ТТОт%'
        OR "group" LIKE 'УКПт%' OR "group" LIKE 'ОМСтр%' OR "group" LIKE 'ОМСфр%'
        OR "group" LIKE 'ТСр%') THEN 'МПН Энергетиков'
    ELSE department
  END
WHERE department IN ('Отделение АиЭС', 'Отделение НГО', 'Отделение СОНХ', 'Отделение ПО', 'Отделение МПН');

-- Остаточные строки «Отделение МПН» (пустая или нераспознанная группа, в продакшене таких нет):
-- относим к МПН Энергетиков, чтобы устаревшее имя полностью исчезло.
UPDATE chats SET department = 'МПН Энергетиков'
WHERE department = 'Отделение МПН';