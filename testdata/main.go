package main

func main() {
	startServer()
}

func startServer() {
	handler := newHandler()
	handler.Listen()
}
