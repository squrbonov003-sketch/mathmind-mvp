package main

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

func main() {
	cfg := loadConfig()
	rand.Seed(time.Now().UnixNano())

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("не удалось подключиться к базе: %v", err)
	}
	defer pool.Close()

	if err := runMigrations(ctx, pool); err != nil {
		log.Fatalf("миграции не применены: %v", err)
	}

	srv := &Server{db: pool, aiAPIKey: cfg.AIAPIKey}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", srv.handleHealth)
	mux.HandleFunc("/auth/teacher/login", srv.handleTeacherLogin)
	mux.HandleFunc("/classes", srv.handleCreateClass)
	mux.HandleFunc("/classes/join", srv.handleJoinClass)
	mux.HandleFunc("/topics", srv.handleTopics)
	mux.HandleFunc("/tasks", srv.handleTasks)
	mux.HandleFunc("/attempts", srv.handleAttempts)
	mux.HandleFunc("/teacher/analytics", srv.handleAnalytics)
	mux.HandleFunc("/teacher/ai", srv.handleAI)

	server := &http.Server{
		Addr:              cfg.Port,
		Handler:           withJSONHeaders(mux),
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("MathMind backend слушает на %s", cfg.Port)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("сервер остановлен с ошибкой: %v", err)
	}
}

type Config struct {
	Port        string
	DatabaseURL string
	AIAPIKey    string
}

func loadConfig() Config {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	if !strings.HasPrefix(port, ":") {
		port = ":" + port
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		host := getenv("DB_HOST", "localhost")
		name := getenv("DB_NAME", "mathmind")
		user := getenv("DB_USER", "mathmind")
		password := getenv("DB_PASSWORD", "mathmind")
		portVal := getenv("DB_PORT", "5432")
		dbURL = fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable", user, password, host, portVal, name)
	}

	return Config{
		Port:        port,
		DatabaseURL: dbURL,
		AIAPIKey:    os.Getenv("AI_API_KEY"),
	}
}

func getenv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}

type Server struct {
	db       *pgxpool.Pool
	aiAPIKey string
}

type apiError struct {
	Error string `json:"error"`
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleTeacherLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, apiError{Error: "только POST"})
		return
	}

	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, apiError{Error: "некорректное тело запроса"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	var teacherID int
	var savedPassword string
	err := s.db.QueryRow(ctx, "SELECT id, password FROM teachers WHERE email=$1", req.Email).Scan(&teacherID, &savedPassword)
	if errors.Is(err, pgx.ErrNoRows) || savedPassword != req.Password {
		writeJSON(w, http.StatusUnauthorized, apiError{Error: "неверный логин или пароль"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, apiError{Error: "ошибка базы"})
		return
	}

	token := fmt.Sprintf("demo-token-%d", teacherID)
	writeJSON(w, http.StatusOK, map[string]any{
		"teacher_id": teacherID,
		"token":      token,
		"message":    "Вход выполнен",
	})
}

func (s *Server) handleCreateClass(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, apiError{Error: "только POST"})
		return
	}
	var req struct {
		Name      string `json:"name"`
		TeacherID int    `json:"teacher_id"`
	}
	if err := decodeJSON(r, &req); err != nil || req.Name == "" || req.TeacherID == 0 {
		writeJSON(w, http.StatusBadRequest, apiError{Error: "укажите name и teacher_id"})
		return
	}

	inviteCode := generateInviteCode()

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	var classID int
	err := s.db.QueryRow(ctx, `INSERT INTO classes (name, teacher_id, invite_code) VALUES ($1, $2, $3)
        RETURNING id`, req.Name, req.TeacherID, inviteCode).Scan(&classID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, apiError{Error: "не удалось создать класс"})
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"class_id":    classID,
		"invite_code": inviteCode,
	})
}

func (s *Server) handleJoinClass(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, apiError{Error: "только POST"})
		return
	}

	var req struct {
		StudentName string `json:"student_name"`
		InviteCode  string `json:"invite_code"`
	}
	if err := decodeJSON(r, &req); err != nil || req.StudentName == "" || req.InviteCode == "" {
		writeJSON(w, http.StatusBadRequest, apiError{Error: "укажите student_name и invite_code"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	var classID int
	err := s.db.QueryRow(ctx, "SELECT id FROM classes WHERE invite_code=$1", strings.ToUpper(req.InviteCode)).Scan(&classID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeJSON(w, http.StatusNotFound, apiError{Error: "класс не найден"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, apiError{Error: "ошибка базы"})
		return
	}

	var studentID int
	err = s.db.QueryRow(ctx, "INSERT INTO students (name) VALUES ($1) RETURNING id", req.StudentName).Scan(&studentID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, apiError{Error: "не удалось сохранить ученика"})
		return
	}

	_, err = s.db.Exec(ctx, "INSERT INTO class_students (class_id, student_id) VALUES ($1, $2) ON CONFLICT DO NOTHING", classID, studentID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, apiError{Error: "не удалось привязать ученика к классу"})
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"class_id":   classID,
		"student_id": studentID,
		"message":    "Ученик добавлен в класс",
	})
}

type topicRow struct {
	ID          int    `json:"id"`
	Title       string `json:"title"`
	Summary     string `json:"summary"`
	TasksCount  int    `json:"tasks_count"`
	PreviewTask string `json:"preview_task"`
}

func (s *Server) handleTopics(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	rows, err := s.db.Query(ctx, `
        SELECT tp.id, tp.title, tp.summary, COALESCE(task_counts.count, 0) AS tasks_count, COALESCE(task_counts.preview, '')
        FROM topics tp
        LEFT JOIN (
          SELECT topic_id, COUNT(*) AS count, MIN(title) AS preview
          FROM tasks
          GROUP BY topic_id
        ) task_counts ON task_counts.topic_id = tp.id
        ORDER BY tp.id`)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, apiError{Error: "ошибка загрузки тем"})
		return
	}
	defer rows.Close()

	var topics []topicRow
	for rows.Next() {
		var t topicRow
		if err := rows.Scan(&t.ID, &t.Title, &t.Summary, &t.TasksCount, &t.PreviewTask); err != nil {
			writeJSON(w, http.StatusInternalServerError, apiError{Error: "ошибка чтения тем"})
			return
		}
		topics = append(topics, t)
	}

	writeJSON(w, http.StatusOK, map[string]any{"topics": topics})
}

type taskResponse struct {
	ID          int                    `json:"id"`
	TopicID     int                    `json:"topic_id"`
	Title       string                 `json:"title"`
	Description string                 `json:"description"`
	Nodes       map[string]nodePayload `json:"nodes"`
	RootNode    string                 `json:"root_node"`
}

type nodePayload struct {
	Key        string          `json:"key"`
	Prompt     string          `json:"prompt"`
	IsTerminal bool            `json:"is_terminal"`
	Choices    []choicePayload `json:"choices"`
}

type choicePayload struct {
	Key         string `json:"key"`
	Text        string `json:"text"`
	NextNodeKey string `json:"next_node_key"`
	IsCorrect   bool   `json:"is_correct"`
	MistakeType string `json:"mistake_type"`
	Hint        string `json:"hint"`
}

func (s *Server) handleTasks(w http.ResponseWriter, r *http.Request) {
	topicID := r.URL.Query().Get("topic_id")
	if topicID == "" {
		writeJSON(w, http.StatusBadRequest, apiError{Error: "укажите topic_id"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()

	taskRows, err := s.db.Query(ctx, `SELECT id, topic_id, title, description, root_node FROM tasks WHERE topic_id=$1 ORDER BY id`, topicID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, apiError{Error: "ошибка загрузки заданий"})
		return
	}
	defer taskRows.Close()

	tasks := make(map[int]*taskResponse)
	var taskIDs []int
	for taskRows.Next() {
		var t taskResponse
		if err := taskRows.Scan(&t.ID, &t.TopicID, &t.Title, &t.Description, &t.RootNode); err != nil {
			writeJSON(w, http.StatusInternalServerError, apiError{Error: "ошибка чтения заданий"})
			return
		}
		t.Nodes = make(map[string]nodePayload)
		tasks[t.ID] = &t
		taskIDs = append(taskIDs, t.ID)
	}

	if len(taskIDs) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{"tasks": []taskResponse{}})
		return
	}

	nodeRows, err := s.db.Query(ctx, `SELECT task_id, node_key, prompt, is_terminal FROM task_nodes WHERE task_id = ANY($1)`, pgx.Array(taskIDs))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, apiError{Error: "ошибка загрузки шагов"})
		return
	}
	defer nodeRows.Close()

	for nodeRows.Next() {
		var taskID int
		var key, prompt string
		var isTerminal bool
		if err := nodeRows.Scan(&taskID, &key, &prompt, &isTerminal); err != nil {
			writeJSON(w, http.StatusInternalServerError, apiError{Error: "ошибка чтения шагов"})
			return
		}
		if t, ok := tasks[taskID]; ok {
			t.Nodes[key] = nodePayload{Key: key, Prompt: prompt, IsTerminal: isTerminal}
		}
	}

	choiceRows, err := s.db.Query(ctx, `SELECT task_id, from_node_key, choice_key, text, next_node_key, is_correct, COALESCE(mistake_type,''), COALESCE(hint,'')
        FROM task_choices WHERE task_id = ANY($1) ORDER BY id`, pgx.Array(taskIDs))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, apiError{Error: "ошибка загрузки вариантов"})
		return
	}
	defer choiceRows.Close()

	for choiceRows.Next() {
		var taskID int
		var fromNode string
		var c choicePayload
		if err := choiceRows.Scan(&taskID, &fromNode, &c.Key, &c.Text, &c.NextNodeKey, &c.IsCorrect, &c.MistakeType, &c.Hint); err != nil {
			writeJSON(w, http.StatusInternalServerError, apiError{Error: "ошибка чтения вариантов"})
			return
		}
		if t, ok := tasks[taskID]; ok {
			node := t.Nodes[fromNode]
			node.Choices = append(node.Choices, c)
			t.Nodes[fromNode] = node
		}
	}

	var resp []taskResponse
	for _, t := range tasks {
		resp = append(resp, *t)
	}

	writeJSON(w, http.StatusOK, map[string]any{"tasks": resp})
}

func (s *Server) handleAttempts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, apiError{Error: "только POST"})
		return
	}

	var req struct {
		AttemptID *int   `json:"attempt_id"`
		StudentID int    `json:"student_id"`
		ClassID   int    `json:"class_id"`
		TaskID    int    `json:"task_id"`
		NodeKey   string `json:"node_key"`
		ChoiceKey string `json:"choice_key"`
	}
	if err := decodeJSON(r, &req); err != nil || req.StudentID == 0 || req.TaskID == 0 || req.NodeKey == "" || req.ChoiceKey == "" {
		writeJSON(w, http.StatusBadRequest, apiError{Error: "проверьте поля student_id, task_id, node_key, choice_key"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()

	var choice choicePayload
	var fromNode string
	err := s.db.QueryRow(ctx, `SELECT from_node_key, choice_key, text, next_node_key, is_correct, COALESCE(mistake_type,''), COALESCE(hint,'')
        FROM task_choices WHERE task_id=$1 AND from_node_key=$2 AND choice_key=$3`, req.TaskID, req.NodeKey, req.ChoiceKey).
		Scan(&fromNode, &choice.Key, &choice.Text, &choice.NextNodeKey, &choice.IsCorrect, &choice.MistakeType, &choice.Hint)
	if errors.Is(err, pgx.ErrNoRows) {
		writeJSON(w, http.StatusBadRequest, apiError{Error: "вариант не найден"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, apiError{Error: "ошибка базы"})
		return
	}

	attemptID := req.AttemptID
	if attemptID == nil {
		var newID int
		err := s.db.QueryRow(ctx, `INSERT INTO attempts (task_id, student_id, class_id) VALUES ($1,$2,$3) RETURNING id`, req.TaskID, req.StudentID, req.ClassID).
			Scan(&newID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, apiError{Error: "не удалось создать попытку"})
			return
		}
		attemptID = &newID
	}

	_, err = s.db.Exec(ctx, `INSERT INTO attempt_steps (attempt_id, node_key, choice_key, is_correct, mistake_type)
        VALUES ($1,$2,$3,$4,$5)`, *attemptID, req.NodeKey, req.ChoiceKey, choice.IsCorrect, nullableString(choice.MistakeType))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, apiError{Error: "не удалось сохранить шаг"})
		return
	}

	completed := false
	if choice.NextNodeKey == "" {
		completed = choice.IsCorrect
	} else {
		var isTerminal bool
		_ = s.db.QueryRow(ctx, "SELECT is_terminal FROM task_nodes WHERE task_id=$1 AND node_key=$2", req.TaskID, choice.NextNodeKey).Scan(&isTerminal)
		completed = choice.IsCorrect && isTerminal
	}

	if completed {
		_, _ = s.db.Exec(ctx, "UPDATE attempts SET completed=true, finished_at=NOW() WHERE id=$1", *attemptID)
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"attempt_id":    *attemptID,
		"is_correct":    choice.IsCorrect,
		"next_node_key": choice.NextNodeKey,
		"mistake_type":  choice.MistakeType,
		"hint":          choice.Hint,
		"completed":     completed,
	})
}

func (s *Server) handleAnalytics(w http.ResponseWriter, r *http.Request) {
	classID := r.URL.Query().Get("class_id")
	if classID == "" {
		writeJSON(w, http.StatusBadRequest, apiError{Error: "укажите class_id"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()

	rows, err := s.db.Query(ctx, `
      SELECT tp.id, tp.title, COALESCE(ts.mistake_type,'') AS mistake_type, COUNT(*) AS total
      FROM attempt_steps ts
      JOIN attempts a ON a.id = ts.attempt_id
      JOIN tasks t ON t.id = a.task_id
      JOIN topics tp ON tp.id = t.topic_id
      WHERE a.class_id = $1 AND ts.is_correct = false
      GROUP BY tp.id, tp.title, mistake_type
      ORDER BY tp.id`, classID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, apiError{Error: "ошибка аналитики"})
		return
	}
	defer rows.Close()

	type summaryRow struct {
		TopicID     int    `json:"topic_id"`
		TopicTitle  string `json:"topic_title"`
		MistakeType string `json:"mistake_type"`
		Count       int    `json:"count"`
	}

	var summary []summaryRow
	for rows.Next() {
		var row summaryRow
		if err := rows.Scan(&row.TopicID, &row.TopicTitle, &row.MistakeType, &row.Count); err != nil {
			writeJSON(w, http.StatusInternalServerError, apiError{Error: "ошибка чтения данных"})
			return
		}
		summary = append(summary, row)
	}

	recentRows, err := s.db.Query(ctx, `
      SELECT ts.id, a.student_id, t.title, ts.node_key, ts.choice_key, ts.is_correct, COALESCE(ts.mistake_type,''), ts.created_at
      FROM attempt_steps ts
      JOIN attempts a ON a.id = ts.attempt_id
      JOIN tasks t ON t.id = a.task_id
      WHERE a.class_id = $1
      ORDER BY ts.created_at DESC
      LIMIT 15`, classID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, apiError{Error: "ошибка истории"})
		return
	}
	defer recentRows.Close()

	type recentRow struct {
		ID        int       `json:"id"`
		StudentID int       `json:"student_id"`
		TaskTitle string    `json:"task_title"`
		NodeKey   string    `json:"node_key"`
		ChoiceKey string    `json:"choice_key"`
		IsCorrect bool      `json:"is_correct"`
		Mistake   string    `json:"mistake_type"`
		CreatedAt time.Time `json:"created_at"`
	}

	var recent []recentRow
	for recentRows.Next() {
		var row recentRow
		if err := recentRows.Scan(&row.ID, &row.StudentID, &row.TaskTitle, &row.NodeKey, &row.ChoiceKey, &row.IsCorrect, &row.Mistake, &row.CreatedAt); err != nil {
			writeJSON(w, http.StatusInternalServerError, apiError{Error: "ошибка чтения истории"})
			return
		}
		recent = append(recent, row)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"mistake_summary": summary,
		"recent_steps":    recent,
	})
}

func (s *Server) handleAI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, apiError{Error: "только POST"})
		return
	}

	var req struct {
		TeacherID int    `json:"teacher_id"`
		ClassID   int    `json:"class_id"`
		TopicID   int    `json:"topic_id"`
		Goal      string `json:"goal"`
		Type      string `json:"type"`
	}
	if err := decodeJSON(r, &req); err != nil || req.TeacherID == 0 || req.Type == "" {
		writeJSON(w, http.StatusBadRequest, apiError{Error: "укажите teacher_id и type"})
		return
	}

	mode := req.Type
	if mode == "" {
		mode = "lesson_plan"
	}

	response := map[string]any{
		"mode":     mode,
		"title":    "Черновик от MathMind",
		"guidance": req.Goal,
		"source":   "stub",
		"sections": []string{
			"Повторение ключевых понятий темы",
			"Пошаговые задачи с подсказками",
			"Домашнее задание с типичными ошибками",
		},
		"prompts": []string{
			"Сформулируй три разноуровневых примера по теме",
			"Подсвети два типичных заблуждения и их исправления",
			"Добавь поощрительное сообщение для учеников",
		},
	}

	if s.aiAPIKey != "" {
		response["source"] = "provider"
		response["note"] = "В продакшене запрос отправляется к внешнему AI-провайдеру по API-ключу"
	}

	writeJSON(w, http.StatusOK, response)
}

// --- helpers ---

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if payload != nil {
		_ = json.NewEncoder(w).Encode(payload)
	}
}

func decodeJSON(r *http.Request, dest any) error {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	return decoder.Decode(dest)
}

func nullableString(v string) any {
	if v == "" {
		return nil
	}
	return v
}

func generateInviteCode() string {
	alphabet := []rune("ABCDEFGHJKLMNPQRSTUVWXYZ23456789")
	b := make([]rune, 6)
	for i := range b {
		b[i] = alphabet[rand.Intn(len(alphabet))]
	}
	return string(b)
}

// --- migrations ---

type migrationFile struct {
	Version string
	Name    string
	SQL     string
}

func runMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	files, err := migrationFiles.ReadDir("migrations")
	if err != nil {
		return err
	}

	var migrations []migrationFile
	for _, f := range files {
		content, err := migrationFiles.ReadFile("migrations/" + f.Name())
		if err != nil {
			return err
		}
		version := strings.SplitN(f.Name(), "_", 2)[0]
		migrations = append(migrations, migrationFile{Version: version, Name: f.Name(), SQL: string(content)})
	}

	sort.Slice(migrations, func(i, j int) bool { return migrations[i].Version < migrations[j].Version })

	if _, err := pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
        version TEXT PRIMARY KEY,
        applied_at TIMESTAMPTZ DEFAULT NOW()
    )`); err != nil {
		return err
	}

	for _, m := range migrations {
		var exists bool
		err := pool.QueryRow(ctx, "SELECT TRUE FROM schema_migrations WHERE version=$1", m.Version).Scan(&exists)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if exists {
			continue
		}

		tx, err := pool.Begin(ctx)
		if err != nil {
			return err
		}

		if _, err := tx.Exec(ctx, m.SQL); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("ошибка миграции %s: %w", m.Name, err)
		}

		if _, err := tx.Exec(ctx, "INSERT INTO schema_migrations (version) VALUES ($1)", m.Version); err != nil {
			_ = tx.Rollback(ctx)
			return err
		}

		if err := tx.Commit(ctx); err != nil {
			return err
		}
	}

	return nil
}

func withJSONHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
