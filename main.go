package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type User struct {
	ID           int
	Name         string
	Email        string
	PasswordHash string
	Role         string
	CreatedAt    time.Time
}

type Class struct {
	ID         int
	Name       string
	InviteCode string
	TeacherID  int
	CreatedAt  time.Time
}

type Concept struct {
	ID          int
	Title       string
	Description string
}

type Choice struct {
	Slug        string
	Text        string
	NextNode    string
	Mistake     bool
	MistakeType string
	Hint        string
}

type Node struct {
	Slug     string
	Title    string
	Prompt   string
	Choices  []Choice
	Terminal bool
}

type Task struct {
	ID          int
	ConceptID   int
	Title       string
	Description string
	RootNode    string
	Nodes       map[string]Node
}

type AttemptStep struct {
	ID          int
	AttemptID   int
	NodeSlug    string
	ChoiceSlug  string
	Mistake     bool
	MistakeType string
	Hint        string
	CreatedAt   time.Time
}

type Attempt struct {
	ID          int
	TaskID      int
	ClassID     int
	StudentID   int
	CurrentNode string
	Completed   bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Steps       []AttemptStep
}

type classMistake struct {
	Type  string
	Count int
}

type studentSummary struct {
	Student User
	Mistake map[string]int
}

type storage struct {
	mu            sync.RWMutex
	users         map[int]User
	classes       map[int]Class
	classStudents map[int][]int
	concepts      map[int]Concept
	tasks         map[int]Task
	attempts      map[int]Attempt
	steps         map[int][]AttemptStep
	counter       map[string]int
}

var (
	memStore    = newStorage()
	baseTmpl    = template.Must(template.New("layout").Parse(layoutTemplate))
	cookieName  = "mathmind_user"
	appPort     = getEnv("PORT", "8080")
	sqlSchema   = strings.TrimSpace(schemaSQL)
	seededTasks = buildSeedTasks()
)

func newStorage() *storage {
	return &storage{
		users:         map[int]User{},
		classes:       map[int]Class{},
		classStudents: map[int][]int{},
		concepts:      map[int]Concept{},
		tasks:         map[int]Task{},
		attempts:      map[int]Attempt{},
		steps:         map[int][]AttemptStep{},
		counter:       map[string]int{},
	}
}

func main() {
	if err := memStore.seed(); err != nil {
		log.Fatalf("seed error: %v", err)
	}

	http.HandleFunc("/", homeHandler)
	http.HandleFunc("/login", loginHandler)
	http.HandleFunc("/logout", logoutHandler)
	http.HandleFunc("/student", studentDashboard)
	http.HandleFunc("/join-class", joinClassHandler)
	http.HandleFunc("/task", studentTaskHandler)
	http.HandleFunc("/choose", chooseHandler)
	http.HandleFunc("/teacher", teacherDashboard)
	http.HandleFunc("/teacher/classes", createClassHandler)
	http.HandleFunc("/teacher/assistant", teacherAssistantHandler)

	log.Printf("Сервер запущен на http://localhost:%s", appPort)
	if err := http.ListenAndServe(":"+appPort, nil); err != nil {
		log.Fatalf("Ошибка сервера: %v", err)
	}
}

func homeHandler(w http.ResponseWriter, r *http.Request) {
	user := memStore.currentUser(r)
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	if user.Role == "teacher" {
		http.Redirect(w, r, "/teacher", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/student", http.StatusSeeOther)
}

func loginHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		data := map[string]string{
			"message": r.URL.Query().Get("msg"),
		}
		if err := baseTmpl.ExecuteTemplate(w, "login", data); err != nil {
			http.Error(w, "не удалось отобразить страницу", http.StatusInternalServerError)
		}
		return
	}

	email := strings.TrimSpace(r.FormValue("email"))
	password := r.FormValue("password")
	user := memStore.authenticate(email, password)
	if user == nil {
		http.Redirect(w, r, "/login?msg=Неверная+пара+логин/пароль", http.StatusSeeOther)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:    cookieName,
		Value:   strconv.Itoa(user.ID),
		Path:    "/",
		Expires: time.Now().Add(24 * time.Hour),
	})

	if user.Role == "teacher" {
		http.Redirect(w, r, "/teacher", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/student", http.StatusSeeOther)
}

func logoutHandler(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:    cookieName,
		Value:   "",
		Path:    "/",
		Expires: time.Now().Add(-1 * time.Hour),
	})
	http.Redirect(w, r, "/login?msg=Вы+вышли+из+системы", http.StatusSeeOther)
}

func studentDashboard(w http.ResponseWriter, r *http.Request) {
	user := memStore.currentUser(r)
	if user == nil || user.Role != "student" {
		http.Redirect(w, r, "/login?msg=Только+для+учеников", http.StatusSeeOther)
		return
	}

	classes := memStore.studentClasses(user.ID)
	concepts := memStore.listConceptsWithTasks()

	data := map[string]any{
		"user":        user,
		"classes":     classes,
		"concepts":    concepts,
		"message":     r.URL.Query().Get("msg"),
		"schema":      sqlSchema,
		"selectedCID": r.URL.Query().Get("class_id"),
	}

	if err := baseTmpl.ExecuteTemplate(w, "student", data); err != nil {
		http.Error(w, "не удалось отобразить страницу", http.StatusInternalServerError)
	}
}

func joinClassHandler(w http.ResponseWriter, r *http.Request) {
	user := memStore.currentUser(r)
	if user == nil || user.Role != "student" {
		http.Redirect(w, r, "/login?msg=Только+для+учеников", http.StatusSeeOther)
		return
	}
	code := strings.TrimSpace(r.FormValue("invite_code"))
	if code == "" {
		http.Redirect(w, r, "/student?msg=Введите+код", http.StatusSeeOther)
		return
	}
	if err := memStore.joinClass(user.ID, code); err != nil {
		http.Redirect(w, r, "/student?msg="+template.URLQueryEscaper(err.Error()), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/student?msg=Класс+добавлен", http.StatusSeeOther)
}

func studentTaskHandler(w http.ResponseWriter, r *http.Request) {
	user := memStore.currentUser(r)
	if user == nil || user.Role != "student" {
		http.Redirect(w, r, "/login?msg=Только+для+учеников", http.StatusSeeOther)
		return
	}
	taskID, _ := strconv.Atoi(r.URL.Query().Get("task_id"))
	classID, _ := strconv.Atoi(r.URL.Query().Get("class_id"))
	if taskID == 0 || classID == 0 {
		http.Redirect(w, r, "/student?msg=Выберите+класс+и+задачу", http.StatusSeeOther)
		return
	}
	if !memStore.isStudentInClass(user.ID, classID) {
		http.Redirect(w, r, "/student?msg=Сначала+присоединитесь+к+классу", http.StatusSeeOther)
		return
	}

	task := memStore.tasks[taskID]
	attempt := memStore.getOrCreateAttempt(user.ID, classID, taskID, task.RootNode)
	node := task.Nodes[attempt.CurrentNode]

	data := map[string]any{
		"user":      user,
		"classID":   classID,
		"task":      task,
		"node":      node,
		"attempt":   attempt,
		"message":   r.URL.Query().Get("msg"),
		"hint":      r.URL.Query().Get("hint"),
		"mistakes":  memStore.recentMistakesForClass(classID),
		"completed": attempt.Completed,
	}

	if err := baseTmpl.ExecuteTemplate(w, "task", data); err != nil {
		http.Error(w, "не удалось отобразить страницу", http.StatusInternalServerError)
	}
}

func chooseHandler(w http.ResponseWriter, r *http.Request) {
	user := memStore.currentUser(r)
	if user == nil || user.Role != "student" {
		http.Redirect(w, r, "/login?msg=Только+для+учеников", http.StatusSeeOther)
		return
	}

	taskID, _ := strconv.Atoi(r.FormValue("task_id"))
	classID, _ := strconv.Atoi(r.FormValue("class_id"))
	choiceSlug := r.FormValue("choice_slug")

	task, ok := memStore.tasks[taskID]
	if !ok {
		http.Redirect(w, r, "/student?msg=Задание+не+найдено", http.StatusSeeOther)
		return
	}
	attempt := memStore.getOrCreateAttempt(user.ID, classID, taskID, task.RootNode)
	node := task.Nodes[attempt.CurrentNode]

	var selected *Choice
	for _, c := range node.Choices {
		c := c
		if c.Slug == choiceSlug {
			selected = &c
			break
		}
	}
	if selected == nil {
		http.Redirect(w, r, fmt.Sprintf("/task?task_id=%d&class_id=%d&msg=Выберите+вариант", taskID, classID), http.StatusSeeOther)
		return
	}

	memStore.recordStep(attempt.ID, selected, node.Slug)

	if selected.Mistake {
		redirect := fmt.Sprintf("/task?task_id=%d&class_id=%d&msg=Попробуйте+ещё+раз&hint=%s", taskID, classID, template.URLQueryEscaper(selected.Hint))
		http.Redirect(w, r, redirect, http.StatusSeeOther)
		return
	}

	next := selected.NextNode
	if next == "" {
		memStore.completeAttempt(attempt.ID)
		http.Redirect(w, r, fmt.Sprintf("/task?task_id=%d&class_id=%d&msg=Готово!+Ответ+найден", taskID, classID), http.StatusSeeOther)
		return
	}

	nextNode := task.Nodes[next]
	if nextNode.Terminal {
		memStore.completeAttempt(attempt.ID)
		http.Redirect(w, r, fmt.Sprintf("/task?task_id=%d&class_id=%d&msg=Задание+завершено", taskID, classID), http.StatusSeeOther)
		return
	}

	memStore.advanceAttempt(attempt.ID, next)
	http.Redirect(w, r, fmt.Sprintf("/task?task_id=%d&class_id=%d&msg=Отлично,+двигаемся+дальше!", taskID, classID), http.StatusSeeOther)
}

func teacherDashboard(w http.ResponseWriter, r *http.Request) {
	user := memStore.currentUser(r)
	if user == nil || user.Role != "teacher" {
		http.Redirect(w, r, "/login?msg=Только+для+учителей", http.StatusSeeOther)
		return
	}
	classID, _ := strconv.Atoi(r.URL.Query().Get("class_id"))
	classes := memStore.teacherClasses(user.ID)
	if classID == 0 && len(classes) > 0 {
		classID = classes[0].ID
	}
	mistakes := memStore.mistakesForClass(classID)
	students := memStore.studentSummary(classID)

	data := map[string]any{
		"user":        user,
		"classes":     classes,
		"selected":    classID,
		"mistakes":    mistakes,
		"students":    students,
		"recentSteps": memStore.recentMistakesForClass(classID),
		"message":     r.URL.Query().Get("msg"),
	}

	if err := baseTmpl.ExecuteTemplate(w, "teacher", data); err != nil {
		http.Error(w, "не удалось отобразить страницу", http.StatusInternalServerError)
	}
}

func createClassHandler(w http.ResponseWriter, r *http.Request) {
	user := memStore.currentUser(r)
	if user == nil || user.Role != "teacher" {
		http.Redirect(w, r, "/login?msg=Только+для+учителей", http.StatusSeeOther)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		http.Redirect(w, r, "/teacher?msg=Название+обязательно", http.StatusSeeOther)
		return
	}
	code := randomCode("CLS")
	memStore.createClass(user.ID, name, code)
	http.Redirect(w, r, fmt.Sprintf("/teacher?msg=Класс+%s+создан,+код+%s", template.URLQueryEscaper(name), code), http.StatusSeeOther)
}

func teacherAssistantHandler(w http.ResponseWriter, r *http.Request) {
	user := memStore.currentUser(r)
	if user == nil || user.Role != "teacher" {
		http.Redirect(w, r, "/login?msg=Только+для+учителей", http.StatusSeeOther)
		return
	}
	classID, _ := strconv.Atoi(r.FormValue("class_id"))
	goal := strings.TrimSpace(r.FormValue("goal"))
	topic := strings.TrimSpace(r.FormValue("topic"))
	result := ""

	if classID != 0 && goal != "" {
		switch goal {
		case "plan":
			result = memStore.generatePlan(classID, topic)
		case "homework":
			result = memStore.generateHomework(classID, topic)
		case "test":
			result = memStore.generateTest(classID, topic)
		case "mistakes":
			result = memStore.explainMistakes(classID, topic)
		default:
			result = "Выберите тип запроса."
		}
	}

	classes := memStore.teacherClasses(user.ID)
	data := map[string]any{
		"user":    user,
		"classes": classes,
		"result":  result,
		"goal":    goal,
		"topic":   topic,
	}

	if err := baseTmpl.ExecuteTemplate(w, "assistant", data); err != nil {
		http.Error(w, "не удалось отобразить страницу", http.StatusInternalServerError)
	}
}

func (s *storage) seed() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.users) > 0 {
		return nil
	}

	now := time.Now()
	teacher := User{ID: s.next("user"), Name: "Учитель", Email: "teacher@mathmind.ru", PasswordHash: hashPassword("teacher123"), Role: "teacher", CreatedAt: now}
	student := User{ID: s.next("user"), Name: "Ученик", Email: "student@mathmind.ru", PasswordHash: hashPassword("student123"), Role: "student", CreatedAt: now}
	s.users[teacher.ID] = teacher
	s.users[student.ID] = student

	concepts := []Concept{
		{ID: s.next("concept"), Title: "Проценты", Description: "Работа с процентами и скидками."},
		{ID: s.next("concept"), Title: "Уравнения", Description: "Линейные уравнения и пропорции."},
	}
	for _, c := range concepts {
		s.concepts[c.ID] = c
	}

	class := Class{ID: s.next("class"), Name: "7А", InviteCode: "START-7A", TeacherID: teacher.ID, CreatedAt: now}
	s.classes[class.ID] = class
	s.classStudents[class.ID] = append(s.classStudents[class.ID], student.ID)

	taskID := 1
	for _, t := range seededTasks {
		t.ID = taskID
		taskID++
		s.tasks[t.ID] = t
	}
	return nil
}

func (s *storage) authenticate(email, password string) *User {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, u := range s.users {
		if strings.EqualFold(u.Email, email) && u.PasswordHash == hashPassword(password) {
			copy := u
			return &copy
		}
	}
	return nil
}

func (s *storage) currentUser(r *http.Request) *User {
	cookie, err := r.Cookie(cookieName)
	if err != nil {
		return nil
	}
	id, _ := strconv.Atoi(cookie.Value)
	if id == 0 {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	u, ok := s.users[id]
	if !ok {
		return nil
	}
	copy := u
	return &copy
}

func (s *storage) studentClasses(studentID int) []Class {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var classes []Class
	for cid, students := range s.classStudents {
		for _, sid := range students {
			if sid == studentID {
				classes = append(classes, s.classes[cid])
			}
		}
	}
	sort.Slice(classes, func(i, j int) bool { return classes[i].ID < classes[j].ID })
	return classes
}

func (s *storage) joinClass(studentID int, inviteCode string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var target *Class
	for _, c := range s.classes {
		if strings.EqualFold(c.InviteCode, inviteCode) {
			c := c
			target = &c
			break
		}
	}
	if target == nil {
		return fmt.Errorf("код не найден")
	}
	students := s.classStudents[target.ID]
	for _, sid := range students {
		if sid == studentID {
			return fmt.Errorf("уже в этом классе")
		}
	}
	s.classStudents[target.ID] = append(s.classStudents[target.ID], studentID)
	return nil
}

func (s *storage) isStudentInClass(studentID, classID int) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, sid := range s.classStudents[classID] {
		if sid == studentID {
			return true
		}
	}
	return false
}

func (s *storage) listConceptsWithTasks() []struct {
	Concept Concept
	Tasks   []Task
} {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []struct {
		Concept Concept
		Tasks   []Task
	}
	for _, c := range s.concepts {
		var tasks []Task
		for _, t := range s.tasks {
			if t.ConceptID == c.ID {
				tasks = append(tasks, t)
			}
		}
		sort.Slice(tasks, func(i, j int) bool { return tasks[i].ID < tasks[j].ID })
		out = append(out, struct {
			Concept Concept
			Tasks   []Task
		}{Concept: c, Tasks: tasks})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Concept.ID < out[j].Concept.ID })
	return out
}

func (s *storage) getOrCreateAttempt(studentID, classID, taskID int, root string) Attempt {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, a := range s.attempts {
		if a.StudentID == studentID && a.ClassID == classID && a.TaskID == taskID {
			return a
		}
	}
	now := time.Now()
	id := s.next("attempt")
	attempt := Attempt{
		ID:          id,
		TaskID:      taskID,
		ClassID:     classID,
		StudentID:   studentID,
		CurrentNode: root,
		Completed:   false,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	s.attempts[id] = attempt
	return attempt
}

func (s *storage) recordStep(attemptID int, choice *Choice, nodeSlug string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	attempt := s.attempts[attemptID]
	step := AttemptStep{
		ID:          s.next("step"),
		AttemptID:   attemptID,
		NodeSlug:    nodeSlug,
		ChoiceSlug:  choice.Slug,
		Mistake:     choice.Mistake,
		MistakeType: choice.MistakeType,
		Hint:        choice.Hint,
		CreatedAt:   time.Now(),
	}
	s.steps[attemptID] = append([]AttemptStep{step}, s.steps[attemptID]...)
	attempt.Steps = append([]AttemptStep{step}, attempt.Steps...)
	if choice.Mistake {
		attempt.UpdatedAt = step.CreatedAt
	}
	s.attempts[attemptID] = attempt
}

func (s *storage) advanceAttempt(attemptID int, nodeSlug string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	attempt := s.attempts[attemptID]
	attempt.CurrentNode = nodeSlug
	attempt.UpdatedAt = time.Now()
	s.attempts[attemptID] = attempt
}

func (s *storage) completeAttempt(attemptID int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	attempt := s.attempts[attemptID]
	attempt.Completed = true
	attempt.UpdatedAt = time.Now()
	s.attempts[attemptID] = attempt
}

func (s *storage) recentMistakesForClass(classID int) []AttemptStep {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var steps []AttemptStep
	for _, stepList := range s.steps {
		for _, st := range stepList {
			attempt := s.attempts[st.AttemptID]
			if attempt.ClassID == classID && st.Mistake {
				steps = append(steps, st)
			}
		}
	}
	sort.Slice(steps, func(i, j int) bool { return steps[i].CreatedAt.After(steps[j].CreatedAt) })
	if len(steps) > 10 {
		steps = steps[:10]
	}
	return steps
}

func (s *storage) mistakesForClass(classID int) []classMistake {
	s.mu.RLock()
	defer s.mu.RUnlock()
	counts := map[string]int{}
	for _, stepList := range s.steps {
		for _, st := range stepList {
			attempt := s.attempts[st.AttemptID]
			if attempt.ClassID == classID && st.Mistake && st.MistakeType != "" {
				counts[st.MistakeType]++
			}
		}
	}
	var result []classMistake
	for k, v := range counts {
		result = append(result, classMistake{Type: k, Count: v})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Count > result[j].Count })
	return result
}

func (s *storage) studentSummary(classID int) []studentSummary {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var students []studentSummary
	for _, sid := range s.classStudents[classID] {
		user := s.users[sid]
		summary := studentSummary{
			Student: user,
			Mistake: map[string]int{},
		}
		for _, attempt := range s.attempts {
			if attempt.ClassID == classID && attempt.StudentID == sid {
				for _, step := range s.steps[attempt.ID] {
					if step.Mistake {
						summary.Mistake[step.MistakeType]++
					}
				}
			}
		}
		students = append(students, summary)
	}
	sort.Slice(students, func(i, j int) bool { return students[i].Student.ID < students[j].Student.ID })
	return students
}

func (s *storage) teacherClasses(teacherID int) []Class {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var classes []Class
	for _, c := range s.classes {
		if c.TeacherID == teacherID {
			classes = append(classes, c)
		}
	}
	sort.Slice(classes, func(i, j int) bool { return classes[i].ID < classes[j].ID })
	return classes
}

func (s *storage) createClass(teacherID int, name, code string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	class := Class{
		ID:         s.next("class"),
		Name:       name,
		InviteCode: code,
		TeacherID:  teacherID,
		CreatedAt:  time.Now(),
	}
	s.classes[class.ID] = class
}

func (s *storage) generatePlan(classID int, topic string) string {
	if topic == "" {
		topic = "Проценты и уравнения"
	}
	mistakes := s.mistakesForClass(classID)
	lines := []string{
		"Цель урока: закрепить " + topic + " через разбор решений.",
		"Структура:",
		"1. Разогрев (5 минут): устные примеры на связь процентов и дробей.",
		"2. Мини-объяснение (10 минут): показать опорный алгоритм решения.",
		"3. Практика (20 минут): ученики проходят 3 задания с выбором шага.",
		"4. Разбор ошибок (10 минут): опираемся на свежую статистику класса.",
	}
	if len(mistakes) > 0 {
		lines = append(lines, "Топ ошибок:")
		lines = append(lines, mistakeLines(mistakes, 3)...)
	}
	return strings.Join(lines, "\n")
}

func (s *storage) generateHomework(classID int, topic string) string {
	if topic == "" {
		topic = "Проценты в бытовых задачах"
	}
	mistakes := s.mistakesForClass(classID)
	lines := []string{
		"Домашняя работа по теме: " + topic,
		"1. Задание: найти цену товара после двух последовательных скидок 10% и 5%.",
		"2. Задание: увеличить число на 12% и объяснить ход действий словами.",
		"3. Задание: составить уравнение по тексту и решить его шагами.",
	}
	if len(mistakes) > 0 {
		lines = append(lines, "Подсказки для учеников:")
		lines = append(lines, mistakeLines(mistakes, 2)...)
	}
	return strings.Join(lines, "\n")
}

func (s *storage) generateTest(classID int, topic string) string {
	if topic == "" {
		topic = "Проценты и линейные уравнения"
	}
	lines := []string{
		"Контрольная работа (2 варианта) по теме: " + topic,
		"Вариант А:",
		"- Найдите итоговую цену товара после скидки 15%.",
		"- Решите уравнение 3x + 12 = 42.",
		"- Опишите, как вы проверяете ответ.",
		"Вариант Б:",
		"- Сколько процентов составляет 45 от 300?",
		"- Решите уравнение 2(x - 5) = 18.",
		"- Объясните, почему выбранные шаги корректны.",
	}
	mistakes := s.mistakesForClass(classID)
	if len(mistakes) > 0 {
		lines = append(lines, "На что обратить внимание при проверке:")
		lines = append(lines, mistakeLines(mistakes, 3)...)
	}
	return strings.Join(lines, "\n")
}

func (s *storage) explainMistakes(classID int, topic string) string {
	mistakes := s.mistakesForClass(classID)
	if len(mistakes) == 0 {
		return "Свежих ошибок пока нет — отличная динамика!"
	}
	var lines []string
	for _, m := range mistakes {
		lines = append(lines, fmt.Sprintf("- %s: встречается %d раз(а). Советуйте ученикам явно проговаривать шаги и проверять размерность.", m.Type, m.Count))
	}
	if topic != "" {
		lines = append(lines, "Контекст темы: "+topic)
	}
	return strings.Join(lines, "\n")
}

func mistakeLines(m []classMistake, limit int) []string {
	var out []string
	for i, item := range m {
		if i >= limit {
			break
		}
		out = append(out, fmt.Sprintf("- %s (%d)", item.Type, item.Count))
	}
	return out
}

func (s *storage) next(key string) int {
	s.counter[key]++
	return s.counter[key]
}

func hashPassword(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func randomCode(prefix string) string {
	b := make([]byte, 3)
	if _, err := rand.Read(b); err != nil {
		return prefix + "-000"
	}
	return fmt.Sprintf("%s-%X", prefix, b)
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

type seedChoice struct {
	Slug        string
	Text        string
	Next        string
	Mistake     bool
	MistakeType string
	Hint        string
}

type seedNode struct {
	Slug     string
	Title    string
	Prompt   string
	Terminal bool
	Choices  []seedChoice
}

type seedTask struct {
	ConceptID   int
	Title       string
	Description string
	Root        string
	Nodes       []seedNode
}

func buildSeedTasks() []Task {
	var tasks []Task
	percentSeeds := []seedTask{
		{
			ConceptID:   1,
			Title:       "Цена со скидкой",
			Description: "Расчёт скидки и итоговой цены.",
			Root:        "start",
			Nodes: []seedNode{
				{
					Slug:   "start",
					Title:  "Шаг 1",
					Prompt: "Скидка 20% на товар за 2500 ₽. Как найти новую цену?",
					Choices: []seedChoice{
						{Slug: "c1", Text: "Найти 20% и вычесть", Next: "calc", Mistake: false},
						{Slug: "c2", Text: "Умножить на 1,2", Next: "start", Mistake: true, MistakeType: "неверный коэффициент", Hint: "Коэффициент 1,2 увеличивает цену."},
						{Slug: "c3", Text: "0,2 от цены — это итог", Next: "start", Mistake: true, MistakeType: "перепутан результат", Hint: "0,2 — это размер скидки, а не новая цена."},
					},
				},
				{
					Slug:   "calc",
					Title:  "Шаг 2",
					Prompt: "Сколько рублей составляет скидка?",
					Choices: []seedChoice{
						{Slug: "c4", Text: "2500 × 0,2 = 500 ₽", Next: "done", Mistake: false},
						{Slug: "c5", Text: "2500 × 0,8 = 2000 ₽", Next: "calc", Mistake: true, MistakeType: "доля перепутана", Hint: "0,8 — это оставшаяся часть, не скидка."},
						{Slug: "c6", Text: "2500 ÷ 5 = 500 ₽, но не понимаю почему", Next: "done", Mistake: false},
					},
				},
				{Slug: "done", Title: "Итог", Prompt: "Итоговая цена: 2000 ₽. Задание завершено.", Terminal: true},
			},
		},
		{
			ConceptID:   1,
			Title:       "НДС 20%",
			Description: "Работаем с налогом на добавленную стоимость.",
			Root:        "start",
			Nodes: []seedNode{
				{
					Slug:   "start",
					Title:  "Шаг 1",
					Prompt: "Есть цена без НДС 1800 ₽. Как добавить НДС 20%?",
					Choices: []seedChoice{
						{Slug: "p1", Text: "Умножить на 1,2", Next: "check", Mistake: false},
						{Slug: "p2", Text: "Прибавить 20 ₽", Next: "start", Mistake: true, MistakeType: "процент к сумме", Hint: "20% — это доля от 1800, а не фиксированная сумма."},
						{Slug: "p3", Text: "Разделить на 0,8", Next: "check", Mistake: false},
					},
				},
				{
					Slug:   "check",
					Title:  "Шаг 2",
					Prompt: "Как проверить ответ?",
					Choices: []seedChoice{
						{Slug: "p4", Text: "Вычесть 20% от результата и сравнить с 1800 ₽", Next: "done", Mistake: false},
						{Slug: "p5", Text: "Вычесть 20 ₽ и получить 1800 ₽", Next: "check", Mistake: true, MistakeType: "неверная проверка", Hint: "Проверка тоже должна учитывать 20% от суммы."},
					},
				},
				{Slug: "done", Title: "Итог", Prompt: "Цена с НДС посчитана и проверена. Отличная работа!", Terminal: true},
			},
		},
		{
			ConceptID:   1,
			Title:       "Рост вклада",
			Description: "Увеличиваем вклад на 12%.",
			Root:        "start",
			Nodes: []seedNode{
				{
					Slug:   "start",
					Title:  "Шаг 1",
					Prompt: "Вклад 10 000 ₽ вырос на 12%. Как найти новый баланс?",
					Choices: []seedChoice{
						{Slug: "g1", Text: "Умножить на 1,12", Next: "explain", Mistake: false},
						{Slug: "g2", Text: "Добавить 120 ₽", Next: "start", Mistake: true, MistakeType: "ошибка масштаба", Hint: "12% от 10 000 — это 1200 ₽, а не 120 ₽."},
						{Slug: "g3", Text: "Прибавить 12 и разделить на 100", Next: "start", Mistake: true, MistakeType: "ошибка в формуле", Hint: "Лучше работать с коэффициентом 1,12."},
					},
				},
				{
					Slug:   "explain",
					Title:  "Шаг 2",
					Prompt: "Как объяснить расчёт однокласснику?",
					Choices: []seedChoice{
						{Slug: "g4", Text: "1,12 = 100% + 12%, умножаем 10 000 × 1,12", Next: "done", Mistake: false},
						{Slug: "g5", Text: "Сначала 10 000 ÷ 12, потом + 10 000", Next: "explain", Mistake: true, MistakeType: "дробление процента", Hint: "Деление на 12 не даёт 12%."},
					},
				},
				{Slug: "done", Title: "Итог", Prompt: "Новый баланс и объяснение готовы.", Terminal: true},
			},
		},
		{
			ConceptID:   1,
			Title:       "Две скидки подряд",
			Description: "Комбинация двух скидок.",
			Root:        "start",
			Nodes: []seedNode{
				{
					Slug:   "start",
					Title:  "Шаг 1",
					Prompt: "Товар 3000 ₽. Сначала скидка 10%, потом ещё 5%. Что делать?",
					Choices: []seedChoice{
						{Slug: "d1", Text: "Применить сначала 0,9, потом 0,95", Next: "check", Mistake: false},
						{Slug: "d2", Text: "Суммировать скидки: 15% сразу", Next: "start", Mistake: true, MistakeType: "ошибка композиции", Hint: "Скидки последовательно, не суммой."},
						{Slug: "d3", Text: "Вычесть 15% от 3000 один раз", Next: "start", Mistake: true, MistakeType: "процент от базы", Hint: "Вторая скидка берётся от уже сниженной цены."},
					},
				},
				{
					Slug:   "check",
					Title:  "Шаг 2",
					Prompt: "Как проверить итог?",
					Choices: []seedChoice{
						{Slug: "d4", Text: "Убедиться, что цена = 3000 × 0,9 × 0,95", Next: "done", Mistake: false},
						{Slug: "d5", Text: "Вычесть 15% ещё раз для проверки", Next: "check", Mistake: true, MistakeType: "повторная скидка", Hint: "Проверка должна сравнивать с исходной ценой."},
					},
				},
				{Slug: "done", Title: "Итог", Prompt: "Комбинированная скидка рассчитана.", Terminal: true},
			},
		},
		{
			ConceptID:   1,
			Title:       "Процент от числа",
			Description: "Находим, сколько процентов составляет часть.",
			Root:        "start",
			Nodes: []seedNode{
				{
					Slug:   "start",
					Title:  "Шаг 1",
					Prompt: "45 это сколько процентов от 300?",
					Choices: []seedChoice{
						{Slug: "s1", Text: "45 ÷ 300 × 100%", Next: "explain", Mistake: false},
						{Slug: "s2", Text: "300 ÷ 45 × 100%", Next: "start", Mistake: true, MistakeType: "перепутаны части", Hint: "Делим часть на целое, не наоборот."},
						{Slug: "s3", Text: "45 × 100%", Next: "start", Mistake: true, MistakeType: "нет деления", Hint: "Проценты — это доля, нужна операция деления."},
					},
				},
				{
					Slug:   "explain",
					Title:  "Шаг 2",
					Prompt: "Как объяснить полученный результат?",
					Choices: []seedChoice{
						{Slug: "s4", Text: "Это 15%, потому что 45/300 = 0,15", Next: "done", Mistake: false},
						{Slug: "s5", Text: "Потому что 45 — это 15 от 300", Next: "done", Mistake: false},
					},
				},
				{Slug: "done", Title: "Итог", Prompt: "Ответ оформлен с пояснением.", Terminal: true},
			},
		},
		{
			ConceptID:   1,
			Title:       "Скидка по купону",
			Description: "Скидка 30% на книгу.",
			Root:        "start",
			Nodes: []seedNode{
				{
					Slug:   "start",
					Title:  "Шаг 1",
					Prompt: "Книга стоит 900 ₽, купон на 30%. Что делаем?",
					Choices: []seedChoice{
						{Slug: "k1", Text: "900 × 0,7", Next: "done", Mistake: false},
						{Slug: "k2", Text: "900 ÷ 0,7", Next: "start", Mistake: true, MistakeType: "деление вместо умножения", Hint: "Доля оставшейся цены — 0,7, нужно умножать."},
						{Slug: "k3", Text: "900 - 30", Next: "start", Mistake: true, MistakeType: "процент как число", Hint: "30 — это не 30% от 900."},
					},
				},
				{Slug: "done", Title: "Итог", Prompt: "Скидка применена корректно.", Terminal: true},
			},
		},
	}

	equationSeeds := []seedTask{
		{
			ConceptID:   2,
			Title:       "Решение 3x + 12 = 42",
			Description: "Базовое линейное уравнение.",
			Root:        "start",
			Nodes: []seedNode{
				{
					Slug:   "start",
					Title:  "Шаг 1",
					Prompt: "3x + 12 = 42. Что делаем сначала?",
					Choices: []seedChoice{
						{Slug: "e1", Text: "Вычесть 12 из обеих частей", Next: "divide", Mistake: false},
						{Slug: "e2", Text: "Разделить 42 на 3 сразу", Next: "start", Mistake: true, MistakeType: "пропуск шага", Hint: "Сначала убери свободный член."},
						{Slug: "e3", Text: "Прибавить 12", Next: "start", Mistake: true, MistakeType: "не туда перенос", Hint: "Нужно убрать 12, а не добавлять."},
					},
				},
				{
					Slug:   "divide",
					Title:  "Шаг 2",
					Prompt: "Получили 3x = 30. Дальше?",
					Choices: []seedChoice{
						{Slug: "e4", Text: "Разделить обе части на 3", Next: "done", Mistake: false},
						{Slug: "e5", Text: "Прибавить 3 к x", Next: "divide", Mistake: true, MistakeType: "неверное действие", Hint: "Чтобы найти x, убери умножение делением."},
					},
				},
				{Slug: "done", Title: "Итог", Prompt: "x = 10. Решение записано.", Terminal: true},
			},
		},
		{
			ConceptID:   2,
			Title:       "Уравнение с дробью",
			Description: "2(x - 5) = 18.",
			Root:        "start",
			Nodes: []seedNode{
				{
					Slug:   "start",
					Title:  "Шаг 1",
					Prompt: "2(x - 5) = 18. Как раскрыть?",
					Choices: []seedChoice{
						{Slug: "q1", Text: "Разделить обе части на 2", Next: "simplify", Mistake: false},
						{Slug: "q2", Text: "Сначала умножить 2 на 5", Next: "simplify", Mistake: false},
						{Slug: "q3", Text: "Отнять 5 с обеих частей", Next: "start", Mistake: true, MistakeType: "неверный порядок", Hint: "Сначала убери коэффициент 2 или раскрой скобки."},
					},
				},
				{
					Slug:   "simplify",
					Title:  "Шаг 2",
					Prompt: "Получили x - 5 = 9. Что дальше?",
					Choices: []seedChoice{
						{Slug: "q4", Text: "Прибавить 5 к обеим частям", Next: "done", Mistake: false},
						{Slug: "q5", Text: "Разделить на 5", Next: "simplify", Mistake: true, MistakeType: "неверная операция", Hint: "Нужно убрать -5 обратным действием."},
					},
				},
				{Slug: "done", Title: "Итог", Prompt: "x = 14. Решение найдено.", Terminal: true},
			},
		},
		{
			ConceptID:   2,
			Title:       "Пропорция",
			Description: "a/5 = 18/30.",
			Root:        "start",
			Nodes: []seedNode{
				{
					Slug:   "start",
					Title:  "Шаг 1",
					Prompt: "a/5 = 18/30. Как найти a?",
					Choices: []seedChoice{
						{Slug: "p1", Text: "Перемножить крест-накрест: 30a = 18×5", Next: "solve", Mistake: false},
						{Slug: "p2", Text: "a = 18 ÷ 30", Next: "start", Mistake: true, MistakeType: "забыт знаменатель", Hint: "Нужно учесть деление на 5."},
						{Slug: "p3", Text: "5a = 18×30", Next: "solve", Mistake: true, MistakeType: "ошибка в кресте", Hint: "Крест-накрест: числитель умножается на противоположный знаменатель."},
					},
				},
				{
					Slug:   "solve",
					Title:  "Шаг 2",
					Prompt: "30a = 90. Что делаем?",
					Choices: []seedChoice{
						{Slug: "p4", Text: "Делим обе части на 30", Next: "done", Mistake: false},
						{Slug: "p5", Text: "Делим на 5", Next: "solve", Mistake: true, MistakeType: "деление на неверное число", Hint: "Чтобы убрать 30, делим на 30."},
					},
				},
				{Slug: "done", Title: "Итог", Prompt: "a = 3. Пропорция решена.", Terminal: true},
			},
		},
		{
			ConceptID:   2,
			Title:       "Перенос в уравнении",
			Description: "5x - 7 = 3x + 9.",
			Root:        "start",
			Nodes: []seedNode{
				{
					Slug:   "start",
					Title:  "Шаг 1",
					Prompt: "5x - 7 = 3x + 9. Что переносим первым?",
					Choices: []seedChoice{
						{Slug: "t1", Text: "Перенести 3x влево, -7 вправо", Next: "calc", Mistake: false},
						{Slug: "t2", Text: "Прибавить 7 к обеим частям только", Next: "start", Mistake: true, MistakeType: "перенесли не всё", Hint: "Нужно собрать x слева, числа справа."},
						{Slug: "t3", Text: "Всё переносим вправо", Next: "start", Mistake: true, MistakeType: "хаотичный перенос", Hint: "Выбери, где будет x, и действуй последовательно."},
					},
				},
				{
					Slug:   "calc",
					Title:  "Шаг 2",
					Prompt: "Получили 2x = 16. Дальше?",
					Choices: []seedChoice{
						{Slug: "t4", Text: "Делим на 2", Next: "done", Mistake: false},
						{Slug: "t5", Text: "Умножаем на 2", Next: "calc", Mistake: true, MistakeType: "неверное действие", Hint: "Чтобы убрать 2x, делим на 2."},
					},
				},
				{Slug: "done", Title: "Итог", Prompt: "x = 8. Уравнение решено.", Terminal: true},
			},
		},
		{
			ConceptID:   2,
			Title:       "Дробные коэффициенты",
			Description: "0,5x + 4 = 9.",
			Root:        "start",
			Nodes: []seedNode{
				{
					Slug:   "start",
					Title:  "Шаг 1",
					Prompt: "0,5x + 4 = 9. Как убрать дробный коэффициент?",
					Choices: []seedChoice{
						{Slug: "f1", Text: "Умножить уравнение на 2", Next: "solve", Mistake: false},
						{Slug: "f2", Text: "Разделить на 0,5 сразу", Next: "solve", Mistake: false},
						{Slug: "f3", Text: "Прибавить 0,5", Next: "start", Mistake: true, MistakeType: "неверное действие", Hint: "Нужно избавиться от умножения 0,5."},
					},
				},
				{
					Slug:   "solve",
					Title:  "Шаг 2",
					Prompt: "После умножения на 2: x + 8 = 18. Что дальше?",
					Choices: []seedChoice{
						{Slug: "f4", Text: "Вычесть 8, получить x = 10", Next: "done", Mistake: false},
						{Slug: "f5", Text: "Разделить на 8", Next: "solve", Mistake: true, MistakeType: "неверная операция", Hint: "Нужно убрать +8."},
					},
				},
				{Slug: "done", Title: "Итог", Prompt: "x найден. Проверка: 0,5×10 + 4 = 9.", Terminal: true},
			},
		},
		{
			ConceptID:   2,
			Title:       "Проверка решения",
			Description: "x/4 + 6 = 10.",
			Root:        "start",
			Nodes: []seedNode{
				{
					Slug:   "start",
					Title:  "Шаг 1",
					Prompt: "x/4 + 6 = 10. Как найти x?",
					Choices: []seedChoice{
						{Slug: "v1", Text: "Вычесть 6, затем умножить на 4", Next: "check", Mistake: false},
						{Slug: "v2", Text: "Умножить на 4, потом вычесть 6", Next: "check", Mistake: false},
						{Slug: "v3", Text: "Разделить 10 на 4", Next: "start", Mistake: true, MistakeType: "ошибка порядка", Hint: "Сначала убери +6, потом деление на 4."},
					},
				},
				{
					Slug:   "check",
					Title:  "Шаг 2",
					Prompt: "Как проверить полученный x?",
					Choices: []seedChoice{
						{Slug: "v4", Text: "Подставить x в исходное уравнение", Next: "done", Mistake: false},
						{Slug: "v5", Text: "Подставить x в любое другое уравнение", Next: "check", Mistake: true, MistakeType: "неверная проверка", Hint: "Проверка делается в исходном выражении."},
					},
				},
				{Slug: "done", Title: "Итог", Prompt: "Решение оформлено с проверкой.", Terminal: true},
			},
		},
	}

	for _, seed := range percentSeeds {
		tasks = append(tasks, convertSeed(seed))
	}
	for _, seed := range equationSeeds {
		tasks = append(tasks, convertSeed(seed))
	}
	return tasks
}

func convertSeed(seed seedTask) Task {
	nodes := map[string]Node{}
	for _, n := range seed.Nodes {
		node := Node{
			Slug:     n.Slug,
			Title:    n.Title,
			Prompt:   n.Prompt,
			Terminal: n.Terminal,
		}
		for _, c := range n.Choices {
			node.Choices = append(node.Choices, Choice{
				Slug:        c.Slug,
				Text:        c.Text,
				NextNode:    c.Next,
				Mistake:     c.Mistake,
				MistakeType: c.MistakeType,
				Hint:        c.Hint,
			})
		}
		nodes[node.Slug] = node
	}
	return Task{
		ConceptID:   seed.ConceptID,
		Title:       seed.Title,
		Description: seed.Description,
		RootNode:    seed.Root,
		Nodes:       nodes,
	}
}

const schemaSQL = `
-- SQL для PostgreSQL (ручной запуск):
-- пользователи
CREATE TABLE IF NOT EXISTS users (
  id SERIAL PRIMARY KEY,
  name TEXT NOT NULL,
  email TEXT NOT NULL UNIQUE,
  password_hash TEXT NOT NULL,
  role TEXT NOT NULL,
  created_at TIMESTAMPTZ DEFAULT now()
);

-- классы
CREATE TABLE IF NOT EXISTS classes (
  id SERIAL PRIMARY KEY,
  name TEXT NOT NULL,
  invite_code TEXT NOT NULL UNIQUE,
  teacher_id INTEGER REFERENCES users(id),
  created_at TIMESTAMPTZ DEFAULT now()
);

-- ученики в классах
CREATE TABLE IF NOT EXISTS class_students (
  class_id INTEGER REFERENCES classes(id),
  student_id INTEGER REFERENCES users(id),
  joined_at TIMESTAMPTZ DEFAULT now(),
  PRIMARY KEY (class_id, student_id)
);

-- понятия и задания
CREATE TABLE IF NOT EXISTS concepts (
  id SERIAL PRIMARY KEY,
  title TEXT NOT NULL,
  description TEXT
);

CREATE TABLE IF NOT EXISTS tasks (
  id SERIAL PRIMARY KEY,
  concept_id INTEGER REFERENCES concepts(id),
  title TEXT NOT NULL,
  description TEXT,
  root_node TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS task_nodes (
  id SERIAL PRIMARY KEY,
  task_id INTEGER REFERENCES tasks(id),
  slug TEXT NOT NULL,
  title TEXT,
  prompt TEXT,
  terminal BOOLEAN DEFAULT FALSE
);

CREATE TABLE IF NOT EXISTS task_choices (
  id SERIAL PRIMARY KEY,
  node_id INTEGER REFERENCES task_nodes(id),
  slug TEXT NOT NULL,
  text TEXT,
  next_node TEXT,
  mistake BOOLEAN DEFAULT FALSE,
  mistake_type TEXT,
  hint TEXT
);

-- попытки и шаги
CREATE TABLE IF NOT EXISTS attempts (
  id SERIAL PRIMARY KEY,
  task_id INTEGER REFERENCES tasks(id),
  class_id INTEGER REFERENCES classes(id),
  student_id INTEGER REFERENCES users(id),
  current_node TEXT,
  completed BOOLEAN DEFAULT FALSE,
  created_at TIMESTAMPTZ DEFAULT now(),
  updated_at TIMESTAMPTZ DEFAULT now()
);

CREATE TABLE IF NOT EXISTS attempt_steps (
  id SERIAL PRIMARY KEY,
  attempt_id INTEGER REFERENCES attempts(id),
  node_slug TEXT,
  choice_slug TEXT,
  mistake BOOLEAN DEFAULT FALSE,
  mistake_type TEXT,
  hint TEXT,
  created_at TIMESTAMPTZ DEFAULT now()
);
`

const layoutTemplate = `
{{define "base_header"}}
<!DOCTYPE html>
<html lang="ru">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>MathMind Teacher+</title>
  <style>
    :root { color-scheme: light; }
    body { font-family: "Inter", system-ui, -apple-system, sans-serif; margin: 0; background: #f5f7fb; color: #0f172a; }
    header { background: linear-gradient(135deg, #4338ca, #6366f1); color: white; padding: 16px; }
    main { padding: 16px; max-width: 1100px; margin: 0 auto; }
    .card { background: white; border-radius: 16px; padding: 16px; box-shadow: 0 10px 30px rgba(0,0,0,0.06); margin-bottom: 16px; }
    .tag { display: inline-block; padding: 4px 12px; border-radius: 999px; background: #eef2ff; color: #4338ca; font-weight: 700; font-size: 12px; }
    nav a { color: white; text-decoration: none; font-weight: 700; margin-right: 12px; }
    nav a.active { text-decoration: underline; }
    .btn { border: none; border-radius: 10px; padding: 10px 12px; cursor: pointer; font-weight: 700; }
    .btn-primary { background: #4338ca; color: white; }
    .btn-secondary { background: #e2e8f0; color: #0f172a; }
    .field { display: block; margin-bottom: 8px; width: 100%; padding: 10px; border-radius: 10px; border: 1px solid #e5e7eb; }
    .grid { display: grid; gap: 12px; }
    .grid-2 { grid-template-columns: repeat(auto-fit, minmax(280px, 1fr)); }
    .metric { background: #eef2ff; border-radius: 12px; padding: 12px; }
    .list { list-style: none; padding: 0; margin: 0; }
    .list li { padding: 10px 0; border-bottom: 1px solid #e2e8f0; }
    .hint { padding: 12px; background: #fff7ed; border: 1px solid #fed7aa; border-radius: 10px; }
    .success { background: #ecfdf3; border: 1px solid #bbf7d0; padding: 12px; border-radius: 10px; }
    table { width: 100%; border-collapse: collapse; }
    th, td { padding: 8px; text-align: left; border-bottom: 1px solid #e2e8f0; }
    .footer { text-align: center; color: #94a3b8; padding: 16px 0 32px 0; }
  </style>
</head>
<body>
  <header>
    <div style="display:flex; align-items:center; justify-content: space-between;">
      <div>
        <div style="font-weight: 800; font-size: 20px;">MathMind Teacher+</div>
        <div style="opacity: 0.85;">Учительский помощник и тренажёр рассуждений</div>
      </div>
      <nav>
        <a href="/student">Ученик</a>
        <a href="/teacher">Учитель</a>
        <a href="/teacher/assistant">AI ассистент</a>
        <a href="/logout">Выход</a>
      </nav>
    </div>
  </header>
  <main>
{{end}}

{{define "base_footer"}}
  </main>
  <div class="footer">MathMind · Данные в памяти процесса · SQL для PostgreSQL в README</div>
</body>
</html>
{{end}}

{{define "login"}}
<!DOCTYPE html>
<html lang="ru">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>MathMind — Вход</title>
  <style>
    body { font-family: "Inter", system-ui; background: #f5f7fb; display:flex; align-items:center; justify-content:center; min-height:100vh; }
    .card { background:white; padding:24px; border-radius:16px; box-shadow:0 10px 30px rgba(0,0,0,0.08); width:360px; }
    .btn { border:none; width:100%; padding:12px; border-radius:10px; background:#4338ca; color:white; font-weight:700; cursor:pointer; }
    .field { width:100%; padding:10px; border-radius:10px; border:1px solid #e5e7eb; margin-bottom:10px; }
    .muted { color:#64748b; font-size:14px; }
  </style>
</head>
<body>
  <div class="card">
    <div style="font-weight:800; font-size:20px; margin-bottom:8px;">MathMind Teacher+</div>
    <div class="muted" style="margin-bottom:16px;">Вход для учителя и ученика</div>
    {{if .message}}
      <div class="hint" style="margin-bottom:12px;">{{.message}}</div>
    {{end}}
    <form method="POST" action="/login">
      <input type="email" name="email" class="field" placeholder="email" required value="teacher@mathmind.ru">
      <input type="password" name="password" class="field" placeholder="пароль" required value="teacher123">
      <button type="submit" class="btn">Войти</button>
    </form>
    <p class="muted" style="margin-top:12px;">Учитель: teacher@mathmind.ru / teacher123<br>Ученик: student@mathmind.ru / student123</p>
  </div>
</body>
</html>
{{end}}

{{define "student"}}
  {{template "base_header"}}
  {{if .message}}
    <div class="card hint">{{.message}}</div>
  {{end}}
  <div class="card">
    <div class="tag">Мои классы</div>
    <div style="display:flex; gap:12px; align-items:center; margin-top:10px;">
      <form method="POST" action="/join-class" style="display:flex; gap:8px; width:100%; max-width:520px;">
        <input type="text" name="invite_code" class="field" placeholder="Код приглашения (например CLS-AB12)" style="margin:0;">
        <button class="btn btn-primary" type="submit">Присоединиться</button>
      </form>
      {{if .classes}}
        <div class="muted">Текущий класс: {{(index .classes 0).Name}}</div>
      {{end}}
    </div>
    {{if not .classes}}
      <p class="muted" style="margin-top:12px;">Класс не выбран. Введите код от учителя.</p>
    {{else}}
      <ul class="list" style="margin-top:12px;">
        {{range .classes}}
          <li><strong>{{.Name}}</strong> · код {{.InviteCode}}</li>
        {{end}}
      </ul>
    {{end}}
  </div>

  <div class="card">
    <div class="tag">Темы и задания</div>
    {{range .concepts}}
      <div style="margin-top:12px;">
        <div style="font-weight:800;">{{.Concept.Title}}</div>
        <div style="color:#475569; margin-bottom:8px;">{{.Concept.Description}}</div>
        <div class="grid grid-2">
          {{range .Tasks}}
            <div class="metric" style="background:#f8fafc;">
              <div style="font-weight:700;">{{.Title}}</div>
              <div style="color:#475569; font-size:14px; margin:4px 0;">{{.Description}}</div>
              {{if $.classes}}
                <div>
                  <a class="btn btn-primary" href="/task?task_id={{.ID}}&class_id={{(index $.classes 0).ID}}">Открыть</a>
                </div>
              {{else}}
                <div class="muted">Сначала присоединитесь к классу</div>
              {{end}}
            </div>
          {{end}}
        </div>
      </div>
    {{end}}
  </div>

  <div class="card">
    <div class="tag">SQL схема (PostgreSQL)</div>
    <pre style="white-space:pre-wrap; background:#0f172a; color:#e2e8f0; padding:12px; border-radius:12px; font-size:12px;">{{.schema}}</pre>
  </div>
  {{template "base_footer"}}
{{end}}

{{define "task"}}
  {{template "base_header"}}
  <div class="card">
    <div class="tag">Задание</div>
    <h2 style="margin:8px 0;">{{.task.Title}}</h2>
    <p style="color:#475569;">{{.task.Description}}</p>

    {{if .message}}
      <div class="hint" style="margin-bottom:12px;">{{.message}}</div>
    {{end}}
    {{if .hint}}
      <div class="hint" style="margin-bottom:12px;"><strong>Подсказка:</strong> {{.hint}}</div>
    {{end}}

    <div class="card" style="padding:12px; background:#f8fafc;">
      <div class="tag">{{.node.Title}}</div>
      <p style="margin:8px 0 12px 0;">{{.node.Prompt}}</p>
      {{if .completed}}
        <div class="success" style="margin-bottom:8px;">Задание завершено</div>
      {{else}}
        <form method="POST" action="/choose" class="grid" style="gap:10px;">
          {{range .node.Choices}}
            <button type="submit" name="choice_slug" value="{{.Slug}}" class="btn btn-secondary" style="width:100%; text-align:left;">{{.Text}}</button>
          {{end}}
          <input type="hidden" name="task_id" value="{{$.task.ID}}">
          <input type="hidden" name="class_id" value="{{$.classID}}">
        </form>
      {{end}}
    </div>
  </div>

  <div class="card">
    <div class="tag">Ошибки в классе</div>
    {{if not .mistakes}}
      <p class="muted">История пуста — пробуйте варианты решения.</p>
    {{else}}
      <ul class="list">
        {{range .mistakes}}
          <li><strong>{{.MistakeType}}</strong> · {{.Hint}}</li>
        {{end}}
      </ul>
    {{end}}
  </div>
  {{template "base_footer"}}
{{end}}

{{define "teacher"}}
  {{template "base_header"}}
  {{if .message}}
    <div class="card hint">{{.message}}</div>
  {{end}}
  <div class="card">
    <div class="tag">Мои классы</div>
    <form method="POST" action="/teacher/classes" style="display:flex; gap:8px; margin-top:10px;">
      <input class="field" style="margin:0;" name="name" placeholder="Новый класс, например 7Б">
      <button class="btn btn-primary" type="submit">Создать класс</button>
    </form>
    <div class="grid grid-2" style="margin-top:12px;">
      {{range .classes}}
        <div class="metric">
          <div style="font-weight:800;">{{.Name}}</div>
          <div style="color:#475569;">Код: {{.InviteCode}}</div>
          <div style="margin-top:6px;">
            <a class="btn btn-secondary" href="/teacher?class_id={{.ID}}">Аналитика</a>
          </div>
        </div>
      {{end}}
    </div>
  </div>

  <div class="card">
    <div class="tag">Топ ошибок по классу</div>
    {{if not .mistakes}}
      <p class="muted">Ошибок ещё нет.</p>
    {{else}}
      <table>
        <thead><tr><th>Тип ошибки</th><th>Повторений</th></tr></thead>
        <tbody>
          {{range .mistakes}}
            <tr><td>{{.Type}}</td><td>{{.Count}}</td></tr>
          {{end}}
        </tbody>
      </table>
    {{end}}
  </div>

  <div class="card">
    <div class="tag">Сводка по ученикам</div>
    {{if not .students}}
      <p class="muted">Нет присоединённых учеников.</p>
    {{else}}
      {{range .students}}
        <div class="metric" style="margin-bottom:8px;">
          <div style="font-weight:700;">{{.Student.Name}}</div>
          {{if not .Mistake}}
            <div class="muted">Ошибок не зафиксировано.</div>
          {{else}}
            <ul class="list">
              {{range $type, $count := .Mistake}}
                <li>{{$type}} — {{$count}}</li>
              {{end}}
            </ul>
          {{end}}
        </div>
      {{end}}
    {{end}}
  </div>

  <div class="card">
    <div class="tag">Последние ошибки</div>
    {{if not .recentSteps}}
      <p class="muted">История пуста.</p>
    {{else}}
      <table>
        <thead><tr><th>Время</th><th>Тип</th><th>Подсказка</th></tr></thead>
        <tbody>
          {{range .recentSteps}}
            <tr><td>{{.CreatedAt.Format "15:04:05"}}</td><td>{{.MistakeType}}</td><td>{{.Hint}}</td></tr>
          {{end}}
        </tbody>
      </table>
    {{end}}
  </div>
  {{template "base_footer"}}
{{end}}

{{define "assistant"}}
  {{template "base_header"}}
  <div class="card">
    <div class="tag">AI ассистент</div>
    <p style="color:#475569;">Генерирует подсказки на основе ошибок класса. Работает локально, без внешних API.</p>
    <form method="POST" action="/teacher/assistant" class="grid grid-2">
      <div>
        <label>Класс</label>
        <select name="class_id" class="field">
          {{range .classes}}
            <option value="{{.ID}}">{{.Name}}</option>
          {{end}}
        </select>
      </div>
      <div>
        <label>Тип запроса</label>
        <select name="goal" class="field">
          <option value="plan">План урока</option>
          <option value="homework">Домашнее задание</option>
          <option value="test">Контрольная (2 варианта)</option>
          <option value="mistakes">Пояснение ошибок</option>
        </select>
      </div>
      <div style="grid-column: span 2;">
        <input class="field" name="topic" placeholder="Тема (опционально)">
      </div>
      <div style="grid-column: span 2;">
        <button class="btn btn-primary" type="submit">Сгенерировать</button>
      </div>
    </form>
    {{if .result}}
      <div class="card" style="margin-top:12px; background:#0f172a; color:#e2e8f0;">
        <pre style="white-space:pre-wrap;">{{.result}}</pre>
      </div>
    {{end}}
  </div>
  {{template "base_footer"}}
{{end}}
`
