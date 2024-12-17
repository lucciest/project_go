package main

import (
	"database/sql"
	"html/template"
	"log"
	"net/http"

	_ "github.com/go-sql-driver/mysql"
	"github.com/gorilla/mux"
	"github.com/gorilla/sessions"
	"golang.org/x/crypto/bcrypt"
)

type Article struct {
	Id                     uint16
	Title, Anons, FullText string
	Author                 uint16
}

type ProfileData struct {
	Username string
	Email    string
	Posts    []Article
}

var posts = []Article{}

var store = sessions.NewCookieStore([]byte("super-secret-key"))

func index(w http.ResponseWriter, r *http.Request) {
	t, err := template.ParseFiles("templates/index.html", "templates/header.html", "templates/footer.html")

	if err != nil {
		http.Error(w, "Ошибка загрузки шаблона", http.StatusInternalServerError)
		return
	}

	db, err := sql.Open("mysql", "root:root@tcp(127.0.0.1:3306)/golang")
	if err != nil {
		panic(err)
	}

	defer db.Close()

	//Выборка данных
	res, err := db.Query("Select *  from `articles`")
	if err != nil {
		panic(err)
	}

	posts = []Article{}

	for res.Next() {
		var post Article
		err = res.Scan(&post.Id, &post.Title, &post.Anons, &post.FullText, &post.Author)
		if err != nil {
			panic(err)
		}

		posts = append(posts, post)
	}

	t.ExecuteTemplate(w, "index", posts)
}

func publication(w http.ResponseWriter, r *http.Request) {
	t, err := template.ParseFiles("templates/publication.html", "templates/header.html", "templates/footer.html")

	if err != nil {
		http.Error(w, "Ошибка рендеринга шаблона", http.StatusInternalServerError)
	}

	t.ExecuteTemplate(w, "publication", nil)
}

func save_article(w http.ResponseWriter, r *http.Request) {
	session, _ := store.Get(r, "session")
	username, ok := session.Values["username"].(string)
	if !ok || username == "" {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	title := r.FormValue("title")
	anons := r.FormValue("anons")
	full_text := r.FormValue("full_text")

	if title == "" || anons == "" || full_text == "" {
		http.Error(w, "Заполните все поля", http.StatusBadRequest)
		return
	}

	db, err := sql.Open("mysql", "root:root@tcp(127.0.0.1:3306)/golang")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	var author_id int
	err = db.QueryRow("SELECT id FROM users WHERE username = ?", username).Scan(&author_id)
	if err != nil {
		http.Error(w, "Ошибка пользователя", http.StatusInternalServerError)
		return
	}

	_, err = db.Exec("INSERT INTO articles (title, anons, full_text, author_id) VALUES (?, ?, ?, ?)", title, anons, full_text, author_id)
	if err != nil {
		http.Error(w, "Ошибка добавления статьи", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/profile", http.StatusSeeOther)
}

func show_post(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)

	t, err := template.ParseFiles("templates/show.html", "templates/header.html", "templates/footer.html")
	if err != nil {
		http.Error(w, "Ошибка загрузки шаблона", http.StatusInternalServerError)
		return
	}

	db, err := sql.Open("mysql", "root:root@tcp(127.0.0.1:3306)/golang")
	if err != nil {
		panic(err)
	}
	defer db.Close()

	// Выборка данных поста и имени автора
	query := `
        SELECT articles.id, articles.title, articles.anons, articles.full_text, users.username
        FROM articles
        JOIN users ON articles.author_id = users.id
        WHERE articles.id = ?`
	row := db.QueryRow(query, vars["id"])

	var post Article
	var authorUsername string
	err = row.Scan(&post.Id, &post.Title, &post.Anons, &post.FullText, &authorUsername)
	if err != nil {
		http.Error(w, "Ошибка загрузки данных", http.StatusInternalServerError)
		return
	}

	// Добавление имени автора в структуру для шаблона
	data := struct {
		Article
		AuthorUsername string
	}{
		Article:        post,
		AuthorUsername: authorUsername,
	}

	t.ExecuteTemplate(w, "show", data)
}

func profile(w http.ResponseWriter, r *http.Request) {
	session, _ := store.Get(r, "session")
	username, ok := session.Values["username"].(string)

	if !ok || username == "" {
		t, err := template.ParseFiles("templates/login.html", "templates/header.html", "templates/footer.html")
		if err != nil {
			http.Error(w, "Ошибка загрузки шаблона", http.StatusInternalServerError)
			return
		}
		t.ExecuteTemplate(w, "login", nil)
		return
	}

	db, err := sql.Open("mysql", "root:root@tcp(127.0.0.1:3306)/golang")
	if err != nil {
		panic(err)
	}
	defer db.Close()

	// Получаем данные пользователя
	var email string
	var userID int
	err = db.QueryRow("SELECT id, email FROM users WHERE username = ?", username).Scan(&userID, &email)
	if err != nil {
		http.Error(w, "Ошибка получения данных пользователя", http.StatusInternalServerError)
		return
	}

	// Получаем посты текущего пользователя
	rows, err := db.Query("SELECT id, title, anons, full_text FROM articles WHERE author_id = ?", userID)
	if err != nil {
		http.Error(w, "Ошибка получения постов", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var userPosts []Article
	for rows.Next() {
		var post Article
		if err := rows.Scan(&post.Id, &post.Title, &post.Anons, &post.FullText); err != nil {
			panic(err)
		}
		userPosts = append(userPosts, post)
	}

	// Рендерим страницу
	data := ProfileData{
		Username: username,
		Email:    email,
		Posts:    userPosts,
	}

	t, _ := template.ParseFiles("templates/profile.html", "templates/header.html", "templates/footer.html")
	t.ExecuteTemplate(w, "profile", data)
}

func login(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		username := r.FormValue("username")
		password := r.FormValue("password")

		db, err := sql.Open("mysql", "root:root@tcp(127.0.0.1:3306)/golang")
		if err != nil {
			http.Error(w, "Ошибка подключения к базе данных", http.StatusInternalServerError)
			return
		}
		defer db.Close()

		var storedPassword string
		err = db.QueryRow("SELECT password FROM users WHERE username = ?", username).Scan(&storedPassword)
		if err != nil {
			http.Error(w, "Неверные данные", http.StatusUnauthorized)
			return
		}

		// Проверка пароля с использованием bcrypt
		err = bcrypt.CompareHashAndPassword([]byte(storedPassword), []byte(password))
		if err != nil {
			http.Error(w, "Неверные данные", http.StatusUnauthorized)
			return
		}

		session, _ := store.Get(r, "session")
		session.Values["username"] = username
		session.Save(r, w)

		http.Redirect(w, r, "/profile", http.StatusSeeOther)
	}
}

func register(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		t, err := template.ParseFiles("templates/register.html", "templates/header.html", "templates/footer.html")
		if err != nil {
			log.Printf("Ошибка загрузки шаблона: %v", err)
			http.Error(w, "Ошибка сервера", http.StatusInternalServerError)
			return
		}
		t.ExecuteTemplate(w, "register", nil)
		return
	}

	if r.Method == http.MethodPost {
		username := r.FormValue("username")
		email := r.FormValue("email")
		password := r.FormValue("password")

		// Проверка на пустые поля
		if username == "" || email == "" || password == "" {
			http.Error(w, "Заполните все поля", http.StatusBadRequest)
			return
		}

		db, err := sql.Open("mysql", "root:root@tcp(127.0.0.1:3306)/golang")
		if err != nil {
			log.Printf("Ошибка подключения к БД: %v", err)
			http.Error(w, "Ошибка сервера", http.StatusInternalServerError)
			return
		}
		defer db.Close()

		// Проверка уникальности username и email
		var exists int
		err = db.QueryRow("SELECT COUNT(*) FROM users WHERE username = ? OR email = ?", username, email).Scan(&exists)
		if err != nil {
			log.Printf("Ошибка при проверке уникальности: %v", err)
			http.Error(w, "Ошибка сервера", http.StatusInternalServerError)
			return
		}
		if exists > 0 {
			http.Error(w, "Пользователь с таким именем или email уже существует", http.StatusConflict)
			return
		}

		// Хеширование пароля
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			log.Printf("Ошибка хеширования пароля: %v", err)
			http.Error(w, "Ошибка сервера", http.StatusInternalServerError)
			return
		}

		_, err = db.Exec("INSERT INTO users (username, email, password) VALUES (?, ?, ?)", username, email, string(hashedPassword))
		if err != nil {
			log.Printf("Ошибка добавления пользователя: %v", err)
			http.Error(w, "Ошибка сервера", http.StatusInternalServerError)
			return
		}

		// Успешная регистрация
		http.Redirect(w, r, "/profile", http.StatusSeeOther)
	}
}

func logout(w http.ResponseWriter, r *http.Request) {
	session, _ := store.Get(r, "session")
	session.Values["username"] = ""
	session.Save(r, w)
	http.Redirect(w, r, "/home", http.StatusSeeOther)
}

func delete_post(w http.ResponseWriter, r *http.Request) {
	session, _ := store.Get(r, "session")
	username, ok := session.Values["username"].(string)
	if !ok || username == "" {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	postID := r.URL.Query().Get("id")
	if postID == "" {
		http.Error(w, "Не указан ID поста", http.StatusBadRequest)
		return
	}

	db, err := sql.Open("mysql", "root:root@tcp(127.0.0.1:3306)/golang")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// Убедимся, что пост принадлежит текущему пользователю
	_, err = db.Exec(`
        DELETE FROM articles 
        WHERE id = ? AND author_id = (
            SELECT id FROM users WHERE username = ?
        )`, postID, username)

	if err != nil {
		http.Error(w, "Ошибка удаления поста", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/profile", http.StatusSeeOther)
}

func edit_post(w http.ResponseWriter, r *http.Request) {
	session, _ := store.Get(r, "session")
	username, ok := session.Values["username"].(string)
	if !ok || username == "" {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	if r.Method == http.MethodGet {
		postID := r.URL.Query().Get("id")
		if postID == "" {
			http.Error(w, "Не указан ID поста", http.StatusBadRequest)
			return
		}

		db, err := sql.Open("mysql", "root:root@tcp(127.0.0.1:3306)/golang")
		if err != nil {
			log.Fatal(err)
		}
		defer db.Close()

		var post Article
		err = db.QueryRow(`
            SELECT articles.id, articles.title, articles.anons, articles.full_text
            FROM articles
            JOIN users ON articles.author_id = users.id
            WHERE articles.id = ? AND users.username = ?`, postID, username).Scan(
			&post.Id, &post.Title, &post.Anons, &post.FullText)
		if err != nil {
			http.Error(w, "Ошибка загрузки данных поста", http.StatusInternalServerError)
			return
		}

		t, err := template.ParseFiles("templates/edit.html", "templates/header.html", "templates/footer.html")
		if err != nil {
			http.Error(w, "Ошибка загрузки шаблона", http.StatusInternalServerError)
			return
		}

		t.ExecuteTemplate(w, "edit", post)
	} else if r.Method == http.MethodPost {
		postID := r.FormValue("id")
		title := r.FormValue("title")
		anons := r.FormValue("anons")
		fullText := r.FormValue("full_text")

		db, err := sql.Open("mysql", "root:root@tcp(127.0.0.1:3306)/golang")
		if err != nil {
			log.Fatal(err)
		}
		defer db.Close()

		_, err = db.Exec(`
            UPDATE articles
            SET title = ?, anons = ?, full_text = ?
            WHERE id = ?`, title, anons, fullText, postID)
		if err != nil {
			http.Error(w, "Ошибка обновления поста", http.StatusInternalServerError)
			return
		}

		http.Redirect(w, r, "/profile", http.StatusSeeOther)
	}
}

func handleFunc() {
	rtr := mux.NewRouter()
	rtr.HandleFunc("/home", index).Methods("GET")
	rtr.HandleFunc("/publication", publication).Methods("GET")
	rtr.HandleFunc("/save_article", save_article).Methods("POST")
	rtr.HandleFunc("/post/{id:[0-9]+}", show_post).Methods("GET")
	rtr.HandleFunc("/profile", profile).Methods("GET", "POST")
	rtr.HandleFunc("/login", login).Methods("POST")
	rtr.HandleFunc("/register", register).Methods("GET", "POST")
	rtr.HandleFunc("/logout", logout).Methods("GET")
	rtr.HandleFunc("/delete_post", delete_post).Methods("GET")
	rtr.HandleFunc("/edit_post", edit_post).Methods("GET", "POST")

	http.Handle("/", rtr)
	log.Println("Сервер запущен на :8080")
	http.ListenAndServe(":8080", nil)
}

func main() {
	handleFunc()
}
