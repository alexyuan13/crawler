package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"golang.org/x/net/html"
)

const NumberOfStories = 60

func crawlRSVPSuccessPage(done chan<- bool) {
	output := make(chan string)

	crawlMap := make(map[string]bool)

	for i := 0; i < NumberOfStories/10; i++ {
		crawlingUrl := fmt.Sprintf("https://web.archive.org/web/20220303193220/https://www.rsvp.com.au/online+dating/testimonials.jsp?start=%d", i*10)
		go crawlStoryUrls(crawlingUrl, output)
	}

	counter := 0
	for s := range output {
		crawlMap[s] = false
		counter++
		if counter == NumberOfStories {

			b, _ := json.Marshal(crawlMap)
			os.WriteFile("storyUrls.json", b, os.ModePerm)
			done <- true
			return
		}
	}
}

func crawlStoryUrls(url string, output chan<- string) {
	resp, err := http.Get(url)
	if err != nil {
		output <- fmt.Sprint(url, ": fail!\n")
		return
	}

	body, err := io.ReadAll(resp.Body)
	defer resp.Body.Close()
	if err != nil {
		fmt.Println(err)
	}

	responseBody := string(body)
	doc, _ := html.Parse(strings.NewReader(responseBody))
	findStoryList(doc, output)
}

func findStoryList(n *html.Node, output chan<- string) {
	if n.Type == html.ElementNode && n.Data == "ul" {
		for _, v := range n.Attr {
			if v.Key == "class" && v.Val == "ui-extracts SS" {
				retreiveStoryNode(n, output)
			}
		}
	}

	for c := n.FirstChild; c != nil; c = c.NextSibling {
		findStoryList(c, output)
	}
}

func retreiveStoryNode(n *html.Node, output chan<- string) {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && c.Data == "li" {
			anchorNode := getChildAnchorNode(c)
			for _, v := range anchorNode.Attr {
				if v.Key == "href" {
					output <- fmt.Sprintf("https://web.archive.org/%s", v.Val)
				}
			}
		}
	}
}

func getChildAnchorNode(n *html.Node) *html.Node {
	var anchorNode *html.Node
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && c.Data == "a" {
			anchorNode = c
		}
	}

	return anchorNode
}

type DataStream struct {
	story Story
	url   string
	fail  bool
}

func crawlRSVPStoryPages(done chan<- bool) {
	stream := make(chan DataStream)
	var numberOfCrawler int

	ufile, err := os.Open(("storyUrls.json"))
	if err != nil {
		fmt.Println("Opening error.")
		return
	}

	crawlMap := map[string]bool{}

	urlJsonParser := json.NewDecoder(ufile)
	if err := urlJsonParser.Decode(&crawlMap); err != nil {
		log.Fatal(err)
	}

	storiesJSON := make(map[string]Story)
	sFile, err := os.Open(("stories.json"))
	if err != nil {
		fmt.Println("stories.json not found.")
	} else {
		storiesJsonParser := json.NewDecoder(sFile)
		if err := storiesJsonParser.Decode(&storiesJSON); err != nil {
			log.Fatal(err)
		}
	}

	for url, done := range crawlMap {
		if !done {
			numberOfCrawler++
			go crawlRSVPStoryPage(url, stream)
		}
	}

	counter := 0
	fmt.Print("numberOfCrawler", numberOfCrawler, "\n")
	for v := range stream {
		counter++
		if !v.fail {
			storiesJSON[v.story.Title] = v.story
			crawlMap[v.url] = true
		}
		if counter == numberOfCrawler {
			s, _ := json.Marshal(storiesJSON)
			u, _ := json.Marshal(crawlMap)
			os.WriteFile("stories.json", s, os.ModePerm)
			os.WriteFile("storyUrls.json", u, os.ModePerm)
			done <- true
		}
	}

}

func crawlRSVPStoryPage(url string, stream chan<- DataStream) {
	client := http.Client{
		Timeout: 5 * time.Second,
	}
	resp, err := client.Get(url)
	if err != nil {
		stream <- DataStream{
			fail: true,
		}
		return
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		stream <- DataStream{
			fail: true,
		}
		return
	}
	defer resp.Body.Close()

	responseBody := string(body)
	doc, err := html.Parse(strings.NewReader(responseBody))
	if err != nil {
		stream <- DataStream{
			fail: true,
		}
		return
	}
	fmt.Println("findStoryArticle")
	articleNode := findStoryArticle(doc, stream, url)
	retrieveStoryJSON(articleNode, stream, url)
}

func findStoryArticle(n *html.Node, stream chan<- DataStream, url string) *html.Node {
	var result *html.Node
	if n.Type == html.ElementNode && n.Data == "article" {
		return n
	}

	if n.NextSibling == nil {
		return nil
	}

	for c := n.FirstChild; c != nil; c = c.NextSibling {
		result = findStoryArticle(c, stream, url)
	}
	return result
}

func retrieveStoryJSON(n *html.Node, stream chan<- DataStream, url string) {
	fmt.Println("retrieveStoryJSON")

	story := Story{}
	paragraphSection := Section{}

	if n == nil {
		stream <- DataStream{
			story: Story{},
			url:   url,
		}
		return
	}

	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && c.Data == "h2" {
			story.Title = c.FirstChild.Data
		}

		if c.Type == html.ElementNode && c.Data == "img" {
			for _, v := range c.Attr {
				if v.Key == "src" {
					imgSrc := v.Val

					// download image
					response, e := http.Get(imgSrc)
					if e != nil {
						stream <- DataStream{
							fail: true,
						}
						return
					}
					defer response.Body.Close()

					fileName := fmt.Sprintf("%s.jpg", story.Title)
					fileName = strings.Replace(fileName, " ", "-", -1)
					os.MkdirAll("images", os.ModePerm)
					file, err := os.Create(fmt.Sprintf("images/%s", fileName))
					if err != nil {
						stream <- DataStream{
							fail: true,
						}
						return
					}
					defer file.Close()

					_, err = io.Copy(file, response.Body)
					if err != nil {
						stream <- DataStream{
							fail: true,
						}
						return
					}

					// generate json
					imageContent := []Content{
						Content{
							ItemType: "image",
							Url:      fmt.Sprintf("/images/dating-hub/success-stories/%s", fileName),
						},
					}
					storySection := Section{
						Anchor:  "",
						Content: imageContent,
					}
					story.Sections = append(story.Sections, storySection)
				}
			}
		}

		if c.Type == html.TextNode && c.Data != "\n" {
			paragraphContent := Content{
				ItemType: "paragraph",
				Children: []ContentChild{
					ContentChild{
						Text: c.Data,
					},
				},
			}

			if story.Summary == "" {
				story.Summary = c.Data
			}
			paragraphSection.Content = append(paragraphSection.Content, paragraphContent)
		}
	}

	story.Sections = append(story.Sections, paragraphSection)
	stream <- DataStream{
		story: story,
		url:   url,
	}
}

type Story struct {
	Title       string    `json:"title"`
	CreatedDate string    `json:"createdDate"`
	Author      string    `json:"author"`
	Summary     string    `json:"summary"`
	Sections    []Section `json:"sections"`
}

type Section struct {
	Anchor  string    `json:"anchor"`
	Content []Content `json:"content"`
}

type Content struct {
	ItemType string         `json:"type"`
	Url      string         `json:"url"`
	Children []ContentChild `json:"children"`
}

type ContentChild struct {
	Text     string         `json:"text"`
	Italic   bool           `json:"italic"`
	Bold     bool           `json:"bold"`
	ItemType string         `json:"type"`
	Url      string         `json:"url"`
	Value    string         `json:"value"`
	Children []ContentChild `json:"children"`
}
