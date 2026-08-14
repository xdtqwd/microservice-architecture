package main

import (
	"log"
	"net/http"
	"order-service/internal/handler"
	"order-service/internal/repository"

	"github.com/gorilla/mux"
)

func main() {
	conn, err := repository.Connect()
	if err != nil {
		log.Fatal("Ошибка Бд:", err)
	}
	defer conn.Close()

	h := handler.New(conn)

	r := mux.NewRouter()
	r.HandleFunc("/products", h.GerProducts).Methods("GET")
	r.HandleFunc("/products/{id}", h.GetProductByID).Methods("GET")
	r.HandleFunc("/orders", h.CreateOrder).Methods("POST")
	r.HandleFunc("/orders", h.GetOrders).Methods("GET")
	r.HandleFunc("/orders/{id}", h.GetOrderByID).Methods("GET")
	r.HandleFunc("/orders/{id}/cancel", h.CancelOrder).Methods("POST")

	log.Println("Order service started on :8083")
	log.Fatal(http.ListenAndServe(":8083", r))
}
