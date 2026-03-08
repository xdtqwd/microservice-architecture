package main

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/mux"
	"golang.org/x/crypto/bcrypt"
)

type User struct {
	ID       int
	UserName string
	Password string
}

var users []User

func main() {
	r := mux.NewRouter()
	r.HandleFunc("/register", registerHandler).Methods("POST")
	r.HandleFunc("/login", loginHandler).Methods("POST")

	log.Println("Users service started on :8081")
	log.Fatal(http.ListenAndServe(":8081", r))

}
func registerHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, "Error", http.StatusInternalServerError)
		return
	}
	user := User{
		ID:       len(users) + 1,
		UserName: req.Username,
		Password: string(hash),
	}
	users = append(users, user)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "registered"})
}

func loginHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	var foundUser *User
	for i, u := range users {
		if u.UserName == req.Username {
			foundUser = &users[i]
			break
		}
	}

	if foundUser == nil {
		http.Error(w, "User not found", http.StatusUnauthorized)
		return
	}

	err := bcrypt.CompareHashAndPassword([]byte(foundUser.Password), []byte(req.Password))
	if err != nil {
		http.Error(w, "Wrong password", http.StatusUnauthorized)
		return
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id":  foundUser.ID,
		"username": foundUser.UserName,
	})

	tokenString, err := token.SignedString([]byte("secret"))
	if err != nil {
		http.Error(w, "Error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"token": tokenString})

}
