package main

func main() {
	done := make(chan bool)
	// go crawlRSVPSuccessPage(done)
	// <-done
	go crawlRSVPStoryPages(done)
	<-done
}
