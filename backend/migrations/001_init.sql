-- Core tables
CREATE TABLE IF NOT EXISTS teachers (
    id SERIAL PRIMARY KEY,
    email TEXT UNIQUE NOT NULL,
    password TEXT NOT NULL,
    name TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS classes (
    id SERIAL PRIMARY KEY,
    teacher_id INTEGER REFERENCES teachers(id),
    name TEXT NOT NULL,
    invite_code TEXT UNIQUE NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS students (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS class_students (
    class_id INTEGER REFERENCES classes(id),
    student_id INTEGER REFERENCES students(id),
    PRIMARY KEY (class_id, student_id)
);

CREATE TABLE IF NOT EXISTS topics (
    id SERIAL PRIMARY KEY,
    title TEXT NOT NULL,
    summary TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS tasks (
    id SERIAL PRIMARY KEY,
    topic_id INTEGER REFERENCES topics(id),
    title TEXT NOT NULL,
    description TEXT NOT NULL,
    root_node TEXT NOT NULL DEFAULT 'start'
);

CREATE TABLE IF NOT EXISTS task_nodes (
    id SERIAL PRIMARY KEY,
    task_id INTEGER REFERENCES tasks(id),
    node_key TEXT NOT NULL,
    prompt TEXT NOT NULL,
    is_terminal BOOLEAN DEFAULT FALSE
);

CREATE TABLE IF NOT EXISTS task_choices (
    id SERIAL PRIMARY KEY,
    task_id INTEGER REFERENCES tasks(id),
    from_node_key TEXT NOT NULL,
    choice_key TEXT NOT NULL,
    text TEXT NOT NULL,
    next_node_key TEXT,
    is_correct BOOLEAN DEFAULT FALSE,
    mistake_type TEXT,
    hint TEXT
);

CREATE TABLE IF NOT EXISTS attempts (
    id SERIAL PRIMARY KEY,
    task_id INTEGER REFERENCES tasks(id),
    student_id INTEGER REFERENCES students(id),
    class_id INTEGER REFERENCES classes(id),
    started_at TIMESTAMPTZ DEFAULT NOW(),
    finished_at TIMESTAMPTZ,
    completed BOOLEAN DEFAULT FALSE
);

CREATE TABLE IF NOT EXISTS attempt_steps (
    id SERIAL PRIMARY KEY,
    attempt_id INTEGER REFERENCES attempts(id),
    node_key TEXT NOT NULL,
    choice_key TEXT NOT NULL,
    is_correct BOOLEAN NOT NULL,
    mistake_type TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Seed teacher and demo class
INSERT INTO teachers (id, email, password, name) VALUES
    (1, 'teacher@mathmind.ru', 'parol123', 'Анна Учитель')
ON CONFLICT (id) DO NOTHING;

INSERT INTO classes (id, teacher_id, name, invite_code) VALUES
    (1, 1, '7А', 'MATH7A')
ON CONFLICT (id) DO NOTHING;

INSERT INTO students (id, name) VALUES
    (1, 'Саша'),
    (2, 'Маша')
ON CONFLICT (id) DO NOTHING;

INSERT INTO class_students (class_id, student_id) VALUES
    (1, 1),
    (1, 2)
ON CONFLICT DO NOTHING;

-- Topics
INSERT INTO topics (id, title, summary) VALUES
    (1, 'Проценты и скидки', 'Как считать скидки, налоги и повышающие коэффициенты'),
    (2, 'Дроби и пропорции', 'Работа с долями, перевод в проценты и сравнение дробей')
ON CONFLICT (id) DO NOTHING;

-- Tasks for topic 1
INSERT INTO tasks (id, topic_id, title, description, root_node) VALUES
    (1, 1, 'Цена со скидкой 20%', 'Определи итоговую цену после скидки в магазине', 'start'),
    (2, 1, 'НДС в чеке', 'Понимаем, как 10% налога влияет на стоимость', 'start'),
    (3, 1, 'Скидка и доставка', 'Сначала скидка 15%, затем фиксированная доставка', 'start'),
    (4, 1, 'Повышение цены', 'Цена выросла на 12%, найдите новую стоимость', 'start'),
    (5, 1, 'Купон на 30%', 'Используем купон и считаем итог', 'start'),
    (6, 1, 'Бонус за объём', 'Скидка 8% при покупке двух товаров', 'start'),
    (7, 2, 'Сравнение дробей', 'Какая дробь больше и почему', 'start'),
    (8, 2, 'Перевод в проценты', 'Долю нужно превратить в проценты', 'start'),
    (9, 2, 'Пропорциональные величины', 'Проверяем, сохраняется ли пропорция', 'start'),
    (10, 2, 'Сложение дробей', 'Складываем дроби с разными знаменателями', 'start'),
    (11, 2, 'Доля от числа', 'Находим, чему равна 3/4 от 120', 'start'),
    (12, 2, 'Скорость и время', 'Используем отношение для расчёта скорости', 'start')
ON CONFLICT (id) DO NOTHING;

-- Nodes (2-3 шага на задачу)
INSERT INTO task_nodes (task_id, node_key, prompt, is_terminal) VALUES
    -- Topic 1
    (1, 'start', 'В магазине скидка 20% на товар за 2500 ₽. Что делаем?', false),
    (1, 'finish', 'Итоговая цена: 2000 ₽. Запомни, скидка уменьшает сумму.', true),

    (2, 'start', 'К цене 1800 ₽ добавляется НДС 10%. Как посчитать?', false),
    (2, 'finish', 'Новая цена: 1980 ₽ (прибавили 10%).', true),

    (3, 'start', 'Цена 2000 ₽, скидка 15% и доставка 200 ₽. С чего начать?', false),
    (3, 'finish', 'Скидка 300 ₽, итог 1700 ₽ с доставкой.', true),

    (4, 'start', 'Цена ноутбука 30000 ₽ выросла на 12%. Как найти новую?', false),
    (4, 'finish', '30000 ₽ × 1,12 = 33600 ₽.', true),

    (5, 'start', 'Купон 30% на товар 1200 ₽. Что считаем первым?', false),
    (5, 'finish', 'Скидка 360 ₽, итог 840 ₽.', true),

    (6, 'start', 'Два одинаковых товара по 900 ₽ со скидкой 8% при покупке двух. Как считать?', false),
    (6, 'finish', 'Общая цена 1800 ₽ × 0,92 = 1656 ₽.', true),

    -- Topic 2
    (7, 'start', 'Какая дробь больше: 3/5 или 4/7?', false),
    (7, 'finish', '3/5 ≈ 0,6 больше, чем 4/7 ≈ 0,57.', true),

    (8, 'start', 'Долю 9/20 нужно перевести в проценты.', false),
    (8, 'finish', '9/20 = 0,45 = 45%.', true),

    (9, 'start', 'Проверка пропорции: 2/3 и 6/9. Эквивалентны ли они?', false),
    (9, 'finish', '2/3 = 0,66… и 6/9 = 0,66…, пропорция сохраняется.', true),

    (10, 'start', 'Сложить 1/4 и 1/6. Что сделать перед сложением?', false),
    (10, 'finish', 'Привести к общему знаменателю 12: получится 5/12.', true),

    (11, 'start', 'Найдите 3/4 от 120.', false),
    (11, 'finish', '120 × 0,75 = 90.', true),

    (12, 'start', 'Машина проехала 150 км за 3 часа. Какая скорость?', false),
    (12, 'finish', '150 км / 3 ч = 50 км/ч.', true);

-- Choices per start node
INSERT INTO task_choices (task_id, from_node_key, choice_key, text, next_node_key, is_correct, mistake_type, hint) VALUES
    -- Task 1
    (1, 'start', 'discount_first', 'Посчитать 20% и вычесть из 2500 ₽', 'finish', true, NULL, 'Скидка равна 0,2 от цены, затем вычитаем.'),
    (1, 'start', 'add_percent', 'Умножить 2500 ₽ на 1,2', 'start', false, 'перепутан знак изменения', 'Коэффициент 1,2 увеличивает цену, а не уменьшает.'),
    (1, 'start', 'only_percent', 'Считать 20% как итоговую цену', 'start', false, 'ошибка в доле', '20% — это только размер скидки, не итог.'),

    -- Task 2
    (2, 'start', 'vat_add', 'Умножить на 1,1 и получить новую цену', 'finish', true, NULL, 'НДС добавляется: цена × 1,1.'),
    (2, 'start', 'vat_subtract', 'Вычесть 10% из 1800 ₽', 'start', false, 'перепутан процесс', 'НДС — надбавка, его добавляют, а не вычитают.'),
    (2, 'start', 'vat_wrong_base', 'Добавить 10% к 10% цены', 'start', false, 'неверная база', '10% считаем от всей суммы, не от части.'),

    -- Task 3
    (3, 'start', 'discount_then_delivery', 'Сначала скидка 15%, потом доставка 200 ₽', 'finish', true, NULL, 'Шаги в правильном порядке.'),
    (3, 'start', 'delivery_first', 'Сначала прибавить доставку, затем скидка', 'start', false, 'смешение этапов', 'Скидку лучше считать от товара, а не от суммы с доставкой.'),
    (3, 'start', 'percent_of_delivery', '15% считать и от доставки', 'start', false, 'неверная база', 'Доставка фиксированная, скидка к ней не применяется.'),

    -- Task 4
    (4, 'start', 'price_increase', 'Умножить на 1,12', 'finish', true, NULL, 'Повышение на 12% — коэффициент 1,12.'),
    (4, 'start', 'add_12', 'Прибавить 12 ₽', 'start', false, 'неверная единица', '12% — это доля цены, не фиксированная сумма.'),
    (4, 'start', 'divide', 'Разделить цену на 0,12', 'start', false, 'операция наоборот', 'Деление на 0,12 увеличит число, это не рост на 12%.'),

    -- Task 5
    (5, 'start', 'coupon_apply', 'Посчитать 30% и вычесть из 1200 ₽', 'finish', true, NULL, 'Купон уменьшает сумму на 0,3 от цены.'),
    (5, 'start', 'coupon_wrong_coeff', 'Умножить на 1,3', 'start', false, 'перепутан знак изменения', '1,3 увеличивает цену. Для скидки нужен коэффициент меньше 1.'),
    (5, 'start', 'coupon_half', 'Считать 15% вместо 30%', 'start', false, 'неправильный процент', 'Используйте точный размер купона — 30%.'),

    -- Task 6
    (6, 'start', 'bulk_discount', 'Считать 8% от общей суммы двух товаров', 'finish', true, NULL, 'Сначала сумма, затем скидка 8%.'),
    (6, 'start', 'single_item', 'Считать скидку 8% только на один товар', 'start', false, 'неверная база', 'Скидка действует на оба товара вместе.'),
    (6, 'start', 'wrong_percent', 'Считать 18% скидки', 'start', false, 'ошибка в проценте', 'Уточни условие: 8% при покупке двух.'),

    -- Task 7
    (7, 'start', 'compare_common', 'Привести к общему знаменателю и сравнить', 'finish', true, NULL, 'Общий знаменатель покажет, что 3/5 больше.'),
    (7, 'start', 'compare_numerators', 'Сравнить только числители 3 и 4', 'start', false, 'игнор знаменателя', 'Нужно учитывать и знаменатель, дроби не с одинаковой базой.'),
    (7, 'start', 'decimal_guess', 'Оценить «на глаз» без вычислений', 'start', false, 'неточного сравнение', 'Лучше привести к десятичным или общему знаменателю.'),

    -- Task 8
    (8, 'start', 'convert_percent', 'Разделить 9 на 20 и умножить на 100%', 'finish', true, NULL, 'Переводим дробь в десятичную и в проценты.'),
    (8, 'start', 'swap_roles', 'Умножить 20 на 9', 'start', false, 'ошибка в действии', 'Сначала делим числитель на знаменатель.'),
    (8, 'start', 'divide_by_100', 'Разделить 9/20 на 100', 'start', false, 'неверный шаг', 'Перевод в проценты — умножение на 100 после деления.'),

    -- Task 9
    (9, 'start', 'reduce_fraction', 'Сократить обе дроби и сравнить', 'finish', true, NULL, 'Обе дроби сводятся к 2/3.'),
    (9, 'start', 'compare_raw', 'Сравнить 2 и 6 напрямую', 'start', false, 'игнор знаменателя', 'Числители не дают картину без знаменателей.'),
    (9, 'start', 'add_values', 'Сложить дроби для проверки', 'start', false, 'лишнее действие', 'Для пропорции нужно сравнить отношение, а не сумму.'),

    -- Task 10
    (10, 'start', 'common_denominator', 'Найти общий знаменатель 12 и сложить', 'finish', true, NULL, 'Общий знаменатель позволит сложить дроби.'),
    (10, 'start', 'add_numerators', 'Сложить числители и знаменатели прямо', 'start', false, 'неверное сложение', 'Так нельзя: знаменатели должны быть общими.'),
    (10, 'start', 'decimal_add', 'Перевести в десятичные и сложить «на глаз»', 'start', false, 'потеря точности', 'Лучше работать с точными дробями.'),

    -- Task 11
    (11, 'start', 'multiply_fraction', 'Умножить 120 на 3/4', 'finish', true, NULL, 'Доля от числа — умножение на дробь.'),
    (11, 'start', 'divide_fraction', 'Разделить 120 на 3/4', 'start', false, 'инверсия действия', 'Деление переворачивает дробь и даёт другой результат.'),
    (11, 'start', 'percent_estimate', 'Прикинуть 75% как 60', 'start', false, 'неверная оценка', '75% от 120 — больше половины, точнее 90.'),

    -- Task 12
    (12, 'start', 'distance_over_time', 'Разделить путь на время', 'finish', true, NULL, 'Скорость = расстояние / время.'),
    (12, 'start', 'time_over_distance', 'Разделить время на путь', 'start', false, 'перепутаны позиции', 'Делитель и делимое перепутаны.'),
    (12, 'start', 'multiply_values', 'Перемножить 150 и 3', 'start', false, 'неверная операция', 'Для скорости нужно деление, не умножение.');

-- Ensure sequences move forward
SELECT setval(pg_get_serial_sequence('teachers','id'), (SELECT MAX(id) FROM teachers));
SELECT setval(pg_get_serial_sequence('classes','id'), (SELECT MAX(id) FROM classes));
SELECT setval(pg_get_serial_sequence('students','id'), (SELECT MAX(id) FROM students));
SELECT setval(pg_get_serial_sequence('topics','id'), (SELECT MAX(id) FROM topics));
SELECT setval(pg_get_serial_sequence('tasks','id'), (SELECT MAX(id) FROM tasks));
SELECT setval(pg_get_serial_sequence('task_nodes','id'), (SELECT MAX(id) FROM task_nodes));
SELECT setval(pg_get_serial_sequence('task_choices','id'), (SELECT MAX(id) FROM task_choices));
