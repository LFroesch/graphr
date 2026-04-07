package main

import "fmt"

type Handler struct {
	port int
}

func newHandler() *Handler {
	return &Handler{port: 8080}
}

func (h *Handler) Listen() {
	fmt.Println("listening on", h.port)
	h.handleRequest()
}

func (h *Handler) handleRequest() {
	body := parseBody()
	validateInput(body)
}

func parseBody() string {
	return "{}"
}

func validateInput(body string) {
	_ = body
}
