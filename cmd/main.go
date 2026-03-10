package main

import (
	crand "crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"golang.org/x/crypto/bcrypt"
)

// ==================== 安全配置常量 ====================
const (
	// 输入长度限制
	MAX_TITLE_LENGTH   = 200
	MAX_CONTENT_LENGTH = 100000
	MAX_USERNAME_LEN   = 50
	MAX_PASSWORD_LEN   = 128

	// 速率限制
	MAX_LOGIN_ATTEMPTS = 5
	LOCKOUT_DURATION   = 15 * time.Minute

	// 密码哈希成本
	BCRYPT_COST = 12
)

// ==================== 全局变量 ====================
var db *sql.DB

// 速率限制：记录登录失败次数
var (
	loginAttempts = make(map[string]*LoginAttempt)
	loginMu       sync.RWMutex
)

type LoginAttempt struct {
	Count     int
	LastFail  time.Time
	LockedUnt time.Time
}

// ==================== 数据结构 ====================
type Document struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Content   string    `json:"content"`
	Author    string    `json:"author"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type LoginResponse struct {
	Success  bool   `json:"success"`
	Token    string `json:"token,omitempty"`
	Username string `json:"username,omitempty"`
	Message  string `json:"message,omitempty"`
}

type DocRequest struct {
	Title   string `json:"title"`
	Content string `json:"content"`
}

type DocResponse struct {
	Success bool       `json:"success"`
	Message string     `json:"message,omitempty"`
	DocID   string     `json:"docId,omitempty"`
	Doc     *Document  `json:"doc,omitempty"`
	Docs    []DocSummary `json:"docs,omitempty"`
}

type DocSummary struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	UpdatedAt string `json:"updatedAt"`
}

// ==================== 安全工具函数 ====================

// hashPassword 使用 bcrypt 加盐哈希密码
func hashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), BCRYPT_COST)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// verifyPassword 验证密码
func verifyPassword(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

// generateToken 生成安全的随机 token
func generateToken() string {
	b := make([]byte, 32)
	_, err := crand.Read(b)
	if err != nil {
		// 降级方案（注意：这不是加密安全的，仅用于紧急情况）
		b = []byte(fmt.Sprintf("%d-%d", time.Now().UnixNano(), rand.Intn(1000000)))
	}
	return hex.EncodeToString(b)
}

// sanitizeInput 清理和验证输入
func sanitizeInput(input string, maxLen int) string {
	// 移除控制字符
	input = strings.Map(func(r rune) rune {
		if r < 32 && r != '\n' && r != '\t' {
			return -1
		}
		return r
	}, input)
	
	// 限制长度
	if len(input) > maxLen {
		input = input[:maxLen]
	}
	
	return strings.TrimSpace(input)
}

// checkRateLimit 检查速率限制
func checkRateLimit(key string) bool {
	loginMu.Lock()
	defer loginMu.Unlock()

	now := time.Now()
	attempt, exists := loginAttempts[key]

	if !exists {
		loginAttempts[key] = &LoginAttempt{Count: 0}
		return true
	}

	// 检查是否已解锁
	if !attempt.LockedUnt.IsZero() && now.After(attempt.LockedUnt) {
		attempt.Count = 0
		attempt.LockedUnt = time.Time{}
		return true
	}

	// 检查是否被锁定
	if !attempt.LockedUnt.IsZero() && now.Before(attempt.LockedUnt) {
		return false
	}

	// 检查失败次数
	if attempt.Count >= MAX_LOGIN_ATTEMPTS {
		attempt.LockedUnt = now.Add(LOCKOUT_DURATION)
		return false
	}

	return true
}

// recordLoginFailure 记录登录失败
func recordLoginFailure(key string) {
	loginMu.Lock()
	defer loginMu.Unlock()

	if _, exists := loginAttempts[key]; !exists {
		loginAttempts[key] = &LoginAttempt{}
	}
	loginAttempts[key].Count++
	loginAttempts[key].LastFail = time.Now()
}

// recordLoginSuccess 记录登录成功
func recordLoginSuccess(key string) {
	loginMu.Lock()
	defer loginMu.Unlock()

	if attempt, exists := loginAttempts[key]; exists {
		attempt.Count = 0
		attempt.LockedUnt = time.Time{}
	}
}

// safeError 返回安全的错误信息（不泄露敏感信息）
func safeError() string {
	return "操作失败，请稍后重试"
}

// ==================== HTTP 处理器 ====================

func main() {
	// 初始化数据库连接
	var err error
	dsn := "docuser:docpass123@tcp(127.0.0.1:3306)/doc_site?charset=utf8mb4&parseTime=True&loc=Local"
	db, err = sql.Open("mysql", dsn)
	if err != nil {
		fmt.Printf("❌ 数据库连接失败：%v\n", err)
		os.Exit(1)
	}

	// 测试数据库连接
	if err := db.Ping(); err != nil {
		fmt.Printf("❌ 数据库 Ping 失败：%v\n", err)
		os.Exit(1)
	}
	fmt.Println("✅ 数据库连接成功")

	// 设置数据库连接池参数
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	// API 路由
	http.HandleFunc("/doc/api/login", rateLimitMiddleware(loginHandler))
	http.HandleFunc("/api/login", rateLimitMiddleware(loginHandler))
	http.HandleFunc("/doc/api/docs", authMiddleware(docsHandler))
	http.HandleFunc("/doc/api/docs/", authMiddleware(docsHandler))
	http.HandleFunc("/api/docs", authMiddleware(docsHandler))
	http.HandleFunc("/api/docs/", authMiddleware(docsHandler))

	// 静态文件服务（仅允许 /doc/ 路径）
	fs := http.FileServer(http.Dir("/var/www/doc-site"))
	http.Handle("/doc/", http.StripPrefix("/doc/", fs))

	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}

	fmt.Printf("🚀 Go 后端服务器已启动\n")
	fmt.Printf("📍 监听端口：%s\n", port)
	fmt.Printf("🔒 安全加固：密码 bcrypt 哈希、速率限制、输入验证\n")

	if err := http.ListenAndServe(":"+port, nil); err != nil {
		fmt.Printf("❌ 服务器错误：%v\n", err)
		os.Exit(1)
	}
}

// rateLimitMiddleware 速率限制中间件
func rateLimitMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 获取客户端标识（IP + User-Agent）
		clientID := r.RemoteAddr + "|" + r.UserAgent()
		
		if !checkRateLimit(clientID) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			json.NewEncoder(w).Encode(LoginResponse{
				Success: false,
				Message: "尝试次数过多，请稍后再试",
			})
			return
		}
		
		next(w, r)
	}
}

// authMiddleware 认证中间件
func authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(DocResponse{Success: false, Message: "未授权"})
			return
		}

		token := strings.TrimPrefix(authHeader, "Bearer ")
		var username string

		err := db.QueryRow("SELECT username FROM users WHERE token = ?", token).Scan(&username)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(DocResponse{Success: false, Message: "无效的 token"})
			return
		}

		// 将用户名存入 header
		r.Header.Set("X-Username", username)
		next(w, r)
	}
}

func loginHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")

	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(LoginResponse{Success: false, Message: "方法不允许"})
		return
	}

	// 限制请求体大小
	r.Body = http.MaxBytesReader(w, r.Body, 1024) // 1KB 限制

	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(LoginResponse{Success: false, Message: "请求过大"})
		return
	}

	var req LoginRequest
	if err := json.Unmarshal(body, &req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(LoginResponse{Success: false, Message: "请求格式错误"})
		return
	}

	// 输入验证
	req.Username = sanitizeInput(req.Username, MAX_USERNAME_LEN)
	req.Password = sanitizeInput(req.Password, MAX_PASSWORD_LEN)

	if req.Username == "" || req.Password == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(LoginResponse{Success: false, Message: "用户名和密码不能为空"})
		return
	}

	// 速率限制检查
	clientID := r.RemoteAddr + "|" + req.Username
	if !checkRateLimit(clientID) {
		w.WriteHeader(http.StatusTooManyRequests)
		json.NewEncoder(w).Encode(LoginResponse{
			Success: false,
			Message: "尝试次数过多，请 15 分钟后再试",
		})
		return
	}

	// 从数据库查询用户
	var storedHash string
	err = db.QueryRow("SELECT password_hash FROM users WHERE username = ?", req.Username).Scan(&storedHash)
	if err != nil {
		recordLoginFailure(clientID)
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(LoginResponse{Success: false, Message: "用户名或密码错误"})
		return
	}

	// 验证密码（bcrypt）
	if !verifyPassword(req.Password, storedHash) {
		recordLoginFailure(clientID)
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(LoginResponse{Success: false, Message: "用户名或密码错误"})
		return
	}

	// 登录成功
	recordLoginSuccess(clientID)

	// 生成新 token
	newToken := generateToken()
	_, err = db.Exec("UPDATE users SET token = ? WHERE username = ?", newToken, req.Username)
	if err != nil {
		fmt.Printf("更新 token 失败：%v\n", err)
	}

	json.NewEncoder(w).Encode(LoginResponse{
		Success:  true,
		Token:    newToken,
		Username: req.Username,
	})
}

func docsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")

	username := r.Header.Get("X-Username")

	// 获取文档 ID（如果有）- 支持两种路径前缀
	path := r.URL.Path
	path = strings.TrimPrefix(path, "/api/docs/")
	path = strings.TrimPrefix(path, "/api/docs")
	path = strings.TrimPrefix(path, "/doc/api/docs/")
	path = strings.TrimPrefix(path, "/doc/api/docs")
	path = strings.TrimSuffix(path, "/")

	switch r.Method {
	case http.MethodGet:
		if path != "" {
			getDoc(w, r, path, username)
		} else {
			listDocs(w, r, username)
		}
	case http.MethodPost:
		saveDoc(w, r, username)
	case http.MethodPut:
		if path == "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(DocResponse{Success: false, Message: "文档 ID 不能为空"})
			return
		}
		updateDoc(w, r, username, path)
	case http.MethodDelete:
		if path == "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(DocResponse{Success: false, Message: "文档 ID 不能为空"})
			return
		}
		deleteDoc(w, r, username, path)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(DocResponse{Success: false, Message: "方法不允许"})
	}
}

func listDocs(w http.ResponseWriter, r *http.Request, username string) {
	rows, err := db.Query("SELECT id, title, updated_at FROM documents WHERE author = ? ORDER BY updated_at DESC", username)
	if err != nil {
		fmt.Printf("数据库查询失败：%v\n", err) // 仅记录到服务器日志
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(DocResponse{Success: false, Message: safeError()})
		return
	}
	defer rows.Close()

	var docList []DocSummary
	for rows.Next() {
		var doc DocSummary
		var updatedAt time.Time
		if err := rows.Scan(&doc.ID, &doc.Title, &updatedAt); err != nil {
			fmt.Printf("Scan error: %v\n", err)
			continue
		}
		doc.UpdatedAt = updatedAt.Format("2006-01-02T15:04:05Z07:00")
		docList = append(docList, doc)
	}

	if docList == nil {
		docList = []DocSummary{}
	}

	json.NewEncoder(w).Encode(DocResponse{Success: true, Docs: docList})
}

func getDoc(w http.ResponseWriter, r *http.Request, docID string, username string) {
	// 验证文档 ID 格式（16 位十六进制）
	if len(docID) != 16 {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(DocResponse{Success: false, Message: "无效的文档 ID"})
		return
	}

	var doc Document
	err := db.QueryRow("SELECT id, title, content, author, created_at, updated_at FROM documents WHERE id = ? AND author = ?", docID, username).Scan(
		&doc.ID, &doc.Title, &doc.Content, &doc.Author, &doc.CreatedAt, &doc.UpdatedAt)
	
	if err == sql.ErrNoRows {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(DocResponse{Success: false, Message: "文档不存在"})
		return
	}
	if err != nil {
		fmt.Printf("数据库查询失败：%v\n", err)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(DocResponse{Success: false, Message: safeError()})
		return
	}

	json.NewEncoder(w).Encode(DocResponse{Success: true, Doc: &doc})
}

func saveDoc(w http.ResponseWriter, r *http.Request, username string) {
	// 限制请求体大小
	r.Body = http.MaxBytesReader(w, r.Body, MAX_CONTENT_LENGTH+1024)

	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(DocResponse{Success: false, Message: "请求过大"})
		return
	}

	var req DocRequest
	if err := json.Unmarshal(body, &req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(DocResponse{Success: false, Message: "请求格式错误"})
		return
	}

	// 输入验证和清理
	req.Title = sanitizeInput(req.Title, MAX_TITLE_LENGTH)
	req.Content = sanitizeInput(req.Content, MAX_CONTENT_LENGTH)

	if req.Title == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(DocResponse{Success: false, Message: "标题不能为空"})
		return
	}

	if req.Content == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(DocResponse{Success: false, Message: "内容不能为空"})
		return
	}

	docID := generateToken()[:16]
	now := time.Now()

	_, err = db.Exec("INSERT INTO documents (id, title, content, author, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)",
		docID, req.Title, req.Content, username, now, now)
	if err != nil {
		fmt.Printf("保存文档失败：%v\n", err)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(DocResponse{Success: false, Message: safeError()})
		return
	}

	json.NewEncoder(w).Encode(DocResponse{
		Success: true,
		Message: "文档已保存",
		DocID:   docID,
	})
}

func updateDoc(w http.ResponseWriter, r *http.Request, username string, docID string) {
	// 验证文档 ID 格式
	if len(docID) != 16 {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(DocResponse{Success: false, Message: "无效的文档 ID"})
		return
	}

	// 限制请求体大小
	r.Body = http.MaxBytesReader(w, r.Body, MAX_CONTENT_LENGTH+1024)

	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(DocResponse{Success: false, Message: "请求过大"})
		return
	}

	var req DocRequest
	if err := json.Unmarshal(body, &req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(DocResponse{Success: false, Message: "请求格式错误"})
		return
	}

	// 输入验证和清理
	req.Title = sanitizeInput(req.Title, MAX_TITLE_LENGTH)
	req.Content = sanitizeInput(req.Content, MAX_CONTENT_LENGTH)

	if req.Title == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(DocResponse{Success: false, Message: "标题不能为空"})
		return
	}

	if req.Content == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(DocResponse{Success: false, Message: "内容不能为空"})
		return
	}

	// 检查文档是否存在且属于当前用户
	var exists int
	err = db.QueryRow("SELECT COUNT(*) FROM documents WHERE id = ? AND author = ?", docID, username).Scan(&exists)
	if err != nil || exists == 0 {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(DocResponse{Success: false, Message: "文档不存在"})
		return
	}

	_, err = db.Exec("UPDATE documents SET title = ?, content = ?, updated_at = ? WHERE id = ? AND author = ?",
		req.Title, req.Content, time.Now(), docID, username)
	if err != nil {
		fmt.Printf("更新文档失败：%v\n", err)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(DocResponse{Success: false, Message: safeError()})
		return
	}

	json.NewEncoder(w).Encode(DocResponse{
		Success: true,
		Message: "文档已更新",
		DocID:   docID,
	})
}

func deleteDoc(w http.ResponseWriter, r *http.Request, username string, docID string) {
	// 验证文档 ID 格式
	if len(docID) != 16 {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(DocResponse{Success: false, Message: "无效的文档 ID"})
		return
	}

	// 检查文档是否存在且属于当前用户
	var exists int
	err := db.QueryRow("SELECT COUNT(*) FROM documents WHERE id = ? AND author = ?", docID, username).Scan(&exists)
	if err != nil || exists == 0 {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(DocResponse{Success: false, Message: "文档不存在"})
		return
	}

	_, err = db.Exec("DELETE FROM documents WHERE id = ? AND author = ?", docID, username)
	if err != nil {
		fmt.Printf("删除文档失败：%v\n", err)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(DocResponse{Success: false, Message: safeError()})
		return
	}

	json.NewEncoder(w).Encode(DocResponse{
		Success: true,
		Message: "文档已删除",
	})
}
