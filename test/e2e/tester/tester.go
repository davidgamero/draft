package main

import (
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
)

func main() {
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		resp, err := client.Get(os.Getenv("URL"))
		if err != nil {
			log.Printf("error sending request: %s", err)
			w.WriteHeader(500)
			return
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Printf("error reading response body: %s", err)
			w.WriteHeader(500)
			return
		}
		fmt.Println("success! body: ", string(body))
	})
	fmt.Println("starting tester for url: ", os.Getenv("URL"))
	healthPort := "8080"
	fmt.Println("listening on port: ", healthPort)
	panic(http.ListenAndServe(fmt.Sprintf(":%s", healthPort), nil))
}
