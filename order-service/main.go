package main

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
)

type Order struct {
	ID        int
	UserId    int
	ProductId int
	Quality   int
}

var orders []Order

func main() {
	r := mux.NewRouter()
	r.HandleFunc("/orders", createOrderHandler).Methods("POST")
	r.HandleFunc("/orders", getOrdersHandler).Methods("GET")
	r.HandleFunc("/orders/{id}", getOrderHandler).Methods("GET")

	log.Println("Order service started on :8083")
	log.Fatal(http.ListenAndServe(":8083", r))
}

func getOrdersHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(orders)
}

func createOrderHandler(w http.ResponseWriter, r *http.Request) {
	var req Order
	json.NewDecoder(r.Body).Decode(&req)
	req.ID = len(orders) + 1
	orders = append(orders, req)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "added"})

}

func getOrderHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, _ := strconv.Atoi(vars["id"])
	for _, p := range orders {
		if p.ID == id {
			w.Header().Set("Content-type", "application/json")
			json.NewEncoder(w).Encode(p)
			return
		}
	}
	http.Error(w, "Product not found", http.StatusNotFound)
}
