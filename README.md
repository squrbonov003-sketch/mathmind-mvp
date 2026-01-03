# MathMind Teacher+

MathMind — это офлайн-MVP тренажёра рассуждений для учеников и панели аналитики для учителя. Фокус: шаги решения, классификация ошибок и готовые подсказки для уроков.

## Что входит
- Go HTTP-сервер без внешних зависимостей (все данные — в памяти процесса).
- Роли: учитель и ученик.
- Учитель:
  - создаёт классы и код приглашения;
  - видит топ ошибок класса, сводку по ученикам и последние неверные шаги;
  - получает локального AI-ассистента (план урока, ДЗ, контрольная в 2 вариантах, пояснение частых ошибок).
- Ученик:
  - входит по аккаунту ученика;
  - присоединяется к классу по коду;
  - решает задания в формате дерева решений с подсказками по типу ошибки;
  - видит историю ошибок класса.
- Темы и задания: 2 концепта (проценты, уравнения), 12 задач, каждая с 2–4 узлами решений и ветками ошибок.
- SQL-схема для PostgreSQL прилагается в интерфейсе и ниже (используются явные SQL-скрипты, без ORM).

## Быстрый старт (локально)
```bash
go run .
```
Откройте:
- Ученик: http://localhost:8080/student
- Учитель: http://localhost:8080/teacher
- AI-ассистент: http://localhost:8080/teacher/assistant

Демо-аккаунты:
- Учитель: `teacher@mathmind.ru` / `teacher123`
- Ученик: `student@mathmind.ru` / `student123`

## Docker / docker-compose
```bash
docker-compose up --build
```
Сервисы:
- `app` — Go-сервер (память процесса). Переменная `PORT` по умолчанию `8080`.
- `db` — PostgreSQL 15 (плейсхолдер для развёртывания). Текущее приложение работает в памяти, но вся схема для PostgreSQL прилагается ниже и в интерфейсе. При желании можно перенести хранилище, выполнив SQL-скрипт и заменив слой сохранения.

## Архитектура
- Go + стандартная библиотека, явные SQL-скрипты для PostgreSQL в `main.go` (константа `schemaSQL`).
- Хранение по умолчанию — в памяти процесса с seed-данными (2 концепта, 12 задач, класс 7А, учитель и ученик).
- Шаги решения фиксируются как `attempt_steps` (mistake_type + hint), что позволяет строить аналитики учителя.
- AI-ассистент работает локально без внешних API: текст генерируется на основе статистики ошибок класса.

## PostgreSQL: схема и перенос данных
SQL-скрипт находится в коде (`schemaSQL`) и отображается на странице ученика. Кратко:
```sql
CREATE TABLE users (
  id SERIAL PRIMARY KEY,
  name TEXT NOT NULL,
  email TEXT NOT NULL UNIQUE,
  password_hash TEXT NOT NULL,
  role TEXT NOT NULL,
  created_at TIMESTAMPTZ DEFAULT now()
);
CREATE TABLE classes (
  id SERIAL PRIMARY KEY,
  name TEXT NOT NULL,
  invite_code TEXT NOT NULL UNIQUE,
  teacher_id INTEGER REFERENCES users(id),
  created_at TIMESTAMPTZ DEFAULT now()
);
CREATE TABLE class_students (
  class_id INTEGER REFERENCES classes(id),
  student_id INTEGER REFERENCES users(id),
  joined_at TIMESTAMPTZ DEFAULT now(),
  PRIMARY KEY (class_id, student_id)
);
CREATE TABLE concepts (
  id SERIAL PRIMARY KEY,
  title TEXT NOT NULL,
  description TEXT
);
CREATE TABLE tasks (
  id SERIAL PRIMARY KEY,
  concept_id INTEGER REFERENCES concepts(id),
  title TEXT NOT NULL,
  description TEXT,
  root_node TEXT NOT NULL
);
CREATE TABLE task_nodes (
  id SERIAL PRIMARY KEY,
  task_id INTEGER REFERENCES tasks(id),
  slug TEXT NOT NULL,
  title TEXT,
  prompt TEXT,
  terminal BOOLEAN DEFAULT FALSE
);
CREATE TABLE task_choices (
  id SERIAL PRIMARY KEY,
  node_id INTEGER REFERENCES task_nodes(id),
  slug TEXT NOT NULL,
  text TEXT,
  next_node TEXT,
  mistake BOOLEAN DEFAULT FALSE,
  mistake_type TEXT,
  hint TEXT
);
CREATE TABLE attempts (
  id SERIAL PRIMARY KEY,
  task_id INTEGER REFERENCES tasks(id),
  class_id INTEGER REFERENCES classes(id),
  student_id INTEGER REFERENCES users(id),
  current_node TEXT,
  completed BOOLEAN DEFAULT FALSE,
  created_at TIMESTAMPTZ DEFAULT now(),
  updated_at TIMESTAMPTZ DEFAULT now()
);
CREATE TABLE attempt_steps (
  id SERIAL PRIMARY KEY,
  attempt_id INTEGER REFERENCES attempts(id),
  node_slug TEXT,
  choice_slug TEXT,
  mistake BOOLEAN DEFAULT FALSE,
  mistake_type TEXT,
  hint TEXT,
  created_at TIMESTAMPTZ DEFAULT now()
);
```

## Ограничения и заметки
- По умолчанию хранилище в памяти. Для прод-окружения подключите PostgreSQL и реализуйте слой сохранения, используя приведённые SQL-таблицы (без ORM).
- Внешних API нет, AI-ассистент работает детерминированно по статистике ошибок.
- UI на Go templates, стили минимальные, всё на русском.

## Проверка
```bash
go test ./...
```
(Тестов нет; команда проверяет сборку.)
