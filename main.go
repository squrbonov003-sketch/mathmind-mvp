package main

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"sync"
	"time"
)

type Choice struct {
	ID          string
	Text        string
	NextNode    string
	Mistake     bool
	MistakeType string
	Hint        string
}

type Node struct {
	ID       string
	Title    string
	Prompt   string
	Choices  []Choice
	Terminal bool
}

type Task struct {
	ID       string
	Title    string
	RootNode string
	Nodes    map[string]Node
}

type AttemptMistake struct {
	AttemptID  string
	TaskTitle  string
	ChoiceText string
	Mistake    string
	Hint       string
	Time       time.Time
}

type Attempt struct {
	ID          string
	TaskID      string
	CurrentNode string
	Completed   bool
	Mistakes    []AttemptMistake
}

type MemoryStore struct {
	mu            sync.Mutex
	attempts      map[string]*Attempt
	mistakeTotals map[string]int
	recent        []AttemptMistake
}

func newMemoryStore() *MemoryStore {
	return &MemoryStore{
		attempts:      make(map[string]*Attempt),
		mistakeTotals: make(map[string]int),
		recent:        make([]AttemptMistake, 0, 10),
	}
}

func (s *MemoryStore) getAttempt(id string) (*Attempt, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	attempt, ok := s.attempts[id]
	return attempt, ok
}

func (s *MemoryStore) createAttempt(taskID string, startNode string, taskTitle string) *Attempt {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := fmt.Sprintf("attempt-%d", time.Now().UnixNano())
	attempt := &Attempt{
		ID:          id,
		TaskID:      taskID,
		CurrentNode: startNode,
	}
	s.attempts[id] = attempt
	return attempt
}

func (s *MemoryStore) recordMistake(attempt *Attempt, record AttemptMistake) {
	s.mu.Lock()
	defer s.mu.Unlock()

	attempt.Mistakes = append(attempt.Mistakes, record)
	s.mistakeTotals[record.Mistake]++
	s.recent = append([]AttemptMistake{record}, s.recent...)
	if len(s.recent) > 10 {
		s.recent = s.recent[:10]
	}
}

func (s *MemoryStore) totals() map[string]int {
	s.mu.Lock()
	defer s.mu.Unlock()

	snapshot := make(map[string]int, len(s.mistakeTotals))
	for k, v := range s.mistakeTotals {
		snapshot[k] = v
	}
	return snapshot
}

func (s *MemoryStore) recentMistakes() []AttemptMistake {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]AttemptMistake, len(s.recent))
	copy(out, s.recent)
	return out
}

var (
	store      = newMemoryStore()
	task       = initTask()
	baseTmpl   = template.Must(template.New("layout").Parse(layoutTemplate))
	cookieName = "mathmind_attempt"
	serverPort = ":8080"
)

func initTask() Task {
	nodes := map[string]Node{
		"start": {
			ID:     "start",
			Title:  "Шаг 1",
			Prompt: "В магазине действует скидка 20% на товар за 2500 ₽. Как найти цену со скидкой?",
			Choices: []Choice{
				{
					ID:          "discount_only",
					Text:        "Посчитать 20% от 2500 ₽ и вычесть эту сумму",
					NextNode:    "discount_step",
					Mistake:     false,
					MistakeType: "",
					Hint:        "",
				},
				{
					ID:          "price_increase",
					Text:        "Умножить 2500 ₽ на 1,2",
					NextNode:    "start",
					Mistake:     true,
					MistakeType: "неверный коэффициент",
					Hint:        "Скидка уменьшает цену. Коэффициент 1,2 увеличивает сумму.",
				},
				{
					ID:          "only_discount",
					Text:        "Умножить 2500 ₽ на 0,2 и это итоговая цена",
					NextNode:    "start",
					Mistake:     true,
					MistakeType: "ошибка в смысле процента",
					Hint:        "0,2 — это только размер скидки. Итоговая цена меньше исходной, но больше 0,2 * 2500.",
				},
			},
		},
		"discount_step": {
			ID:     "discount_step",
			Title:  "Шаг 2",
			Prompt: "Сколько рублей составляет скидка?",
			Choices: []Choice{
				{
					ID:          "discount_correct",
					Text:        "2500 ₽ × 0,2 = 500 ₽",
					NextNode:    "final_price",
					Mistake:     false,
					MistakeType: "",
					Hint:        "",
				},
				{
					ID:          "discount_wrong_multiply",
					Text:        "2500 ₽ × 0,8 = 2000 ₽",
					NextNode:    "discount_step",
					Mistake:     true,
					MistakeType: "перепутан процент",
					Hint:        "0,8 — это доля оставшейся цены. Скидка — это 20% или коэффициент 0,2.",
				},
				{
					ID:          "discount_wrong_divide",
					Text:        "2500 ₽ ÷ 5 = 500 ₽, но это 5%",
					NextNode:    "discount_step",
					Mistake:     true,
					MistakeType: "ошибка в вычислении",
					Hint:        "20% — это одна пятая. Деление на 5 даёт сумму скидки, но важно понять, почему это 20%.",
				},
			},
		},
		"final_price": {
			ID:       "final_price",
			Title:    "Шаг 3",
			Prompt:   "Цена со скидкой: 2500 ₽ − 500 ₽ = 2000 ₽. Задание выполнено!",
			Choices:  nil,
			Terminal: true,
		},
	}

	return Task{
		ID:       "discount-task",
		Title:    "Цена со скидкой",
		RootNode: "start",
		Nodes:    nodes,
	}
}

type studentPageData struct {
	Task           Task
	Attempt        *Attempt
	Node           Node
	Message        string
	Hint           string
	MistakeTotals  map[string]int
	RecentMistakes []AttemptMistake
	Completed      bool
}

func main() {
	http.HandleFunc("/", studentHandler)
	http.HandleFunc("/choose", chooseHandler)
	http.HandleFunc("/reset", resetHandler)
	http.HandleFunc("/teacher", teacherHandler)

	log.Printf("Сервер запущен на http://localhost%s", serverPort)
	if err := http.ListenAndServe(serverPort, nil); err != nil {
		log.Fatalf("Ошибка сервера: %v", err)
	}
}

func studentHandler(w http.ResponseWriter, r *http.Request) {
	attempt := loadAttempt(w, r)
	node := task.Nodes[attempt.CurrentNode]

	data := studentPageData{
		Task:           task,
		Attempt:        attempt,
		Node:           node,
		Message:        r.URL.Query().Get("msg"),
		Hint:           r.URL.Query().Get("hint"),
		MistakeTotals:  store.totals(),
		RecentMistakes: store.recentMistakes(),
		Completed:      attempt.Completed,
	}

	if err := baseTmpl.ExecuteTemplate(w, "student", data); err != nil {
		http.Error(w, "не удалось отобразить страницу", http.StatusInternalServerError)
	}
}

func chooseHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	attempt := loadAttempt(w, r)
	node := task.Nodes[attempt.CurrentNode]
	choiceID := r.FormValue("choice_id")

	var selectedChoice *Choice
	for _, c := range node.Choices {
		c := c
		if c.ID == choiceID {
			selectedChoice = &c
			break
		}
	}

	if selectedChoice == nil {
		http.Redirect(w, r, "/?msg=Выберите+вариант", http.StatusSeeOther)
		return
	}

	if selectedChoice.Mistake {
		record := AttemptMistake{
			AttemptID:  attempt.ID,
			TaskTitle:  task.Title,
			ChoiceText: selectedChoice.Text,
			Mistake:    selectedChoice.MistakeType,
			Hint:       selectedChoice.Hint,
			Time:       time.Now(),
		}
		store.recordMistake(attempt, record)
		redirectWithHint(w, r, selectedChoice.Hint)
		return
	}

	if selectedChoice.NextNode == "" {
		attempt.Completed = true
	} else {
		attempt.CurrentNode = selectedChoice.NextNode
		nextNode := task.Nodes[selectedChoice.NextNode]
		if nextNode.Terminal {
			attempt.Completed = true
		}
	}

	http.Redirect(w, r, "/?msg=Отлично,+двигаемся+дальше!", http.StatusSeeOther)
}

func resetHandler(w http.ResponseWriter, r *http.Request) {
	newAttempt := store.createAttempt(task.ID, task.RootNode, task.Title)
	http.SetCookie(w, &http.Cookie{
		Name:    cookieName,
		Value:   newAttempt.ID,
		Path:    "/",
		Expires: time.Now().Add(24 * time.Hour),
	})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func teacherHandler(w http.ResponseWriter, r *http.Request) {
	data := studentPageData{
		Task:           task,
		MistakeTotals:  store.totals(),
		RecentMistakes: store.recentMistakes(),
	}
	if err := baseTmpl.ExecuteTemplate(w, "teacher", data); err != nil {
		http.Error(w, "не удалось отобразить страницу", http.StatusInternalServerError)
	}
}

func loadAttempt(w http.ResponseWriter, r *http.Request) *Attempt {
	cookie, err := r.Cookie(cookieName)
	if err == nil {
		if attempt, ok := store.getAttempt(cookie.Value); ok {
			return attempt
		}
	}
	attempt := store.createAttempt(task.ID, task.RootNode, task.Title)
	http.SetCookie(w, &http.Cookie{
		Name:    cookieName,
		Value:   attempt.ID,
		Path:    "/",
		Expires: time.Now().Add(24 * time.Hour),
	})
	return attempt
}

func redirectWithHint(w http.ResponseWriter, r *http.Request, hint string) {
	http.Redirect(w, r, "/?msg=Попробуйте+ещё+раз&hint="+template.URLQueryEscaper(hint), http.StatusSeeOther)
}

const layoutTemplate = `
{{define "header"}}
<!DOCTYPE html>
<html lang="ru">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>MathMind</title>
  <style>
    body { font-family: "Inter", system-ui, sans-serif; margin: 0; padding: 0; background: #f6f6fb; color: #0f172a; }
    header { background: #4338ca; color: white; padding: 16px; }
    main { padding: 16px; max-width: 900px; margin: 0 auto; }
    .card { background: white; border-radius: 12px; padding: 16px; box-shadow: 0 8px 20px rgba(0,0,0,0.06); margin-bottom: 16px; }
    .choices { display: grid; gap: 12px; }
    .choice-btn { display: block; width: 100%; text-align: left; padding: 12px; border-radius: 10px; border: 1px solid #e5e7eb; background: #f9fafb; cursor: pointer; font-size: 16px; }
    .choice-btn:hover { border-color: #4338ca; }
    .hint { padding: 12px; background: #fff7ed; border: 1px solid #fed7aa; border-radius: 10px; }
    .tag { display: inline-block; padding: 4px 10px; border-radius: 12px; background: #eef2ff; color: #4338ca; font-weight: 600; font-size: 12px; }
    .metrics { display: grid; grid-template-columns: repeat(auto-fit, minmax(180px, 1fr)); gap: 12px; }
    .metric { background: #eef2ff; color: #1e1b4b; padding: 12px; border-radius: 10px; }
    nav a { color: white; margin-right: 12px; text-decoration: none; font-weight: 600; }
    .footer { text-align: center; color: #64748b; padding: 12px; font-size: 14px; }
    .btn-secondary { display: inline-block; padding: 10px 12px; border-radius: 10px; border: 1px solid #cbd5e1; text-decoration: none; color: #0f172a; background: #f8fafc; }
    .btn-primary { display: inline-block; padding: 10px 12px; border-radius: 10px; border: none; background: #4338ca; color: white; cursor: pointer; }
  </style>
</head>
<body>
  <header>
    <div style="display:flex; align-items:center; justify-content: space-between;">
      <div>
        <div style="font-weight: 700; font-size: 20px;">MathMind — тренажёр рассуждений</div>
        <div style="opacity: 0.85;">Учим шаги решения, а не только ответ</div>
      </div>
      <nav>
        <a href="/">Ученик</a>
        <a href="/teacher">Учитель</a>
      </nav>
    </div>
  </header>
  <main>
{{end}}

{{define "footer"}}
  </main>
  <div class="footer">MathMind MVP · Вся логика хранится в памяти сервера</div>
</body>
</html>
{{end}}

{{define "student"}}
  {{template "header"}}
  <div class="card">
    <div class="tag">{{.Task.Title}}</div>
    <h2 style="margin: 8px 0;">{{.Node.Title}}</h2>
    <p style="margin: 8px 0 16px 0;">{{.Node.Prompt}}</p>

    {{if .Message}}
      <div class="hint" style="margin-bottom: 12px;">{{.Message}}</div>
    {{end}}
    {{if .Hint}}
      <div class="hint" style="margin-bottom: 12px;"><strong>Подсказка:</strong> {{.Hint}}</div>
    {{end}}

    {{if .Completed}}
      <div class="hint" style="background:#ecfdf3; border-color:#bbf7d0; margin-bottom:12px;">
        Отличная работа! Вы дошли до конца решения.
      </div>
      <a class="btn-secondary" href="/reset">Начать заново</a>
    {{else}}
      <form action="/choose" method="POST" class="choices">
        {{range .Node.Choices}}
          <button class="choice-btn" type="submit" name="choice_id" value="{{.ID}}">{{.Text}}</button>
        {{end}}
      </form>
    {{end}}
  </div>

  <div class="card">
    <div class="tag">Мини-аналитика</div>
    <div class="metrics" style="margin-top: 12px;">
      {{if not .MistakeTotals}}
        <div class="metric">Ошибок пока нет — продолжайте!</div>
      {{else}}
        {{range $type, $count := .MistakeTotals}}
          <div class="metric">
            <div style="font-weight:700;">{{$type}}</div>
            <div style="font-size:32px; font-weight:800;">{{$count}}</div>
            <div style="color:#475569;">сколько раз встречалась</div>
          </div>
        {{end}}
      {{end}}
    </div>
  </div>

  <div class="card">
    <div class="tag">Последние ошибки</div>
    {{if not .RecentMistakes}}
      <p style="margin-top:12px;">История пока пустая.</p>
    {{else}}
      <ul style="list-style:none; padding:0; margin:12px 0 0 0;">
        {{range .RecentMistakes}}
          <li style="padding:10px 8px; border-bottom:1px solid #e2e8f0;">
            <div style="font-weight:700;">{{.Mistake}}</div>
            <div style="color:#475569; margin:4px 0;">{{.ChoiceText}}</div>
            <div style="font-size:13px; color:#94a3b8;">{{.Time.Format "15:04:05"}} · {{.Hint}}</div>
          </li>
        {{end}}
      </ul>
    {{end}}
  </div>
  {{template "footer"}}
{{end}}

{{define "teacher"}}
  {{template "header"}}
  <div class="card">
    <div class="tag">Учитель</div>
    <h2 style="margin: 8px 0;">Сводка по ошибкам</h2>
    <p style="margin: 8px 0 16px 0;">Данные в памяти сервера. Обновите страницу после новых попыток.</p>

    <div class="metrics">
      {{if not .MistakeTotals}}
        <div class="metric">Ошибок пока нет — ученики молодцы!</div>
      {{else}}
        {{range $type, $count := .MistakeTotals}}
          <div class="metric">
            <div style="font-weight:700;">{{$type}}</div>
            <div style="font-size:32px; font-weight:800;">{{$count}}</div>
            <div style="color:#475569;">фиксируется у учеников</div>
          </div>
        {{end}}
      {{end}}
    </div>
  </div>

  <div class="card">
    <div class="tag">История</div>
    {{if not .RecentMistakes}}
      <p style="margin-top:12px;">История пуста. Попросите ученика пройти задание.</p>
    {{else}}
      <table style="width:100%; border-collapse: collapse; margin-top: 12px;">
        <thead>
          <tr style="text-align:left; border-bottom:1px solid #e2e8f0;">
            <th style="padding:8px;">Время</th>
            <th style="padding:8px;">Тип ошибки</th>
            <th style="padding:8px;">Шаг ученика</th>
            <th style="padding:8px;">Подсказка</th>
          </tr>
        </thead>
        <tbody>
          {{range .RecentMistakes}}
            <tr style="border-bottom:1px solid #e2e8f0;">
              <td style="padding:8px; color:#475569;">{{.Time.Format "15:04:05"}} </td>
              <td style="padding:8px; font-weight:700;">{{.Mistake}}</td>
              <td style="padding:8px;">{{.ChoiceText}}</td>
              <td style="padding:8px; color:#0f172a;">{{.Hint}}</td>
            </tr>
          {{end}}
        </tbody>
      </table>
    {{end}}
  </div>
  {{template "footer"}}
{{end}}
`
