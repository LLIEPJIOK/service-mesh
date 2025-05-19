package main

import (
	"fmt"
	"log"
	"net/http"
)

var cnt int32

func counterHandler(w http.ResponseWriter, r *http.Request) {
	cnt++
	// 2 из 3 запросов возвращают ошибку
	// таким образом идёт проверка на повторы в sidecar
	if cnt%3 != 0 {
		http.Error(
			w,
			http.StatusText(http.StatusTooManyRequests),
			http.StatusTooManyRequests,
		)

		return
	}

	fmt.Fprint(w, cnt)
}

func main() {
	// запуск сервера
	http.HandleFunc("/", counterHandler)

	fmt.Println("Server is running on http://localhost:8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatal(err)
	}
}
