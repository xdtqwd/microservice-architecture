package main

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
)

type Product struct {
	ID    int
	Name  string
	Price float64
}

var products []Product

func main() {
	r := mux.NewRouter()
	r.HandleFunc("/products", getProductsHandler).Methods("GET")
	r.HandleFunc("/products", addProductHandler).Methods("POST")
	r.HandleFunc("/products/{id}", getProductHandler).Methods("GET")
	r.HandleFunc("/products/{id}", deleteProductHandler).Methods("DELETE")
	log.Println("Product service started on :8082")
	log.Fatal(http.ListenAndServe(":8082", r))
}

func getProductsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(products)
}
func addProductHandler(w http.ResponseWriter, r *http.Request) {
	var req Product
	json.NewDecoder(r.Body).Decode(&req)

	req.ID = len(products) + 1
	products = append(products, req)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "added"})
}

func getProductHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, _ := strconv.Atoi(vars["id"])
	for _, p := range products {
		if p.ID == id {
			w.Header().Set("Content-type", "application/json")
			json.NewEncoder(w).Encode(p)
			return
		}
	}
	http.Error(w, "Product not found", http.StatusNotFound)
}

func deleteProductHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, _ := strconv.Atoi(vars["id"])
	for i, p := range products {
		if p.ID == id {
			products = append(products[:i], products[i+1:]...)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"message": "delete"})
			return
		}
	}
	http.Error(w, "Product not found", http.StatusNotFound)
}
