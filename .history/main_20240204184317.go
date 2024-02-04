package main

import (
	"bytes"
	"html/template"
	"log"
	"math"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/joho/godotenv"

	"github.com/freshman-tech/news-demo-starter-files/news"
)

var tpl = template.Must(template.ParseFiles("index.html"))

type Search struct {
	Query        string
	NextPage     int
	TotalPages   int
	Results      *news.Results
	Loading      bool
	Error        bool
	ErrorMessage string
}

func (s *Search) IsLastPage() bool {
	return s.NextPage >= s.TotalPages
}

func (s *Search) CurrentPage() int {
	if s.NextPage == 1 {
		return s.NextPage
	}
	return s.NextPage - 1
}

func (s *Search) PreviousPage() int {
	return s.CurrentPage() - 1
}

func indexHandler(w http.ResponseWriter, r *http.Request) {
	buf := &bytes.Buffer{}
	err := tpl.Execute(buf, nil)
	if err != nil {
		log.Printf("Error rendering index template: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	buf.WriteTo(w)
}

func searchHandler(newsapi *news.Client, searchCache *sync.Map) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, err := url.Parse(r.URL.String())
		if err != nil {
			log.Printf("Error parsing URL: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		params := u.Query()
		searchQuery := params.Get("q")
		page := params.Get("page")
		if page == "" {
			page = "1"
		}

		cachedResults, cached := searchCache.Load(searchQuery)
		if cached {
			log.Printf("Using cached results for query: %s", searchQuery)
			results := cachedResults.(*news.Results)
			nextPage, err := strconv.Atoi(page)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			search := &Search{
				Query:      searchQuery,
				NextPage:   nextPage,
				TotalPages: int(math.Ceil(float64(results.TotalResults) / float64(newsapi.PageSize))),
				Results:    results,
			}

			if ok := !search.IsLastPage(); ok {
				search.NextPage++
			}

			buf := &bytes.Buffer{}
			err = tpl.Execute(buf, search)
			if err != nil {
				log.Printf("Error rendering search template: %v", err)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}

			buf.WriteTo(w)
			return
		}

		results, err := newsapi.FetchEverything(searchQuery, page)
		if err != nil {
			log.Printf("Error fetching news: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		nextPage, err := strconv.Atoi(page)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		search := &Search{
			Query:      searchQuery,
			NextPage:   nextPage,
			TotalPages: int(math.Ceil(float64(results.TotalResults) / float64(newsapi.PageSize))),
			Results:    results,
		}

		if ok := !search.IsLastPage(); ok {
			search.NextPage++
		}

		searchCache.Store(searchQuery, results)
		buf := &bytes.Buffer{}
		err = tpl.Execute(buf, search)
		if err != nil {
			log.Printf("Error rendering search template: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		buf.WriteTo(w)
	}
}

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Println("Error loading .env file")
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "3005"
	}

	apiKey := os.Getenv("NEWS_API_KEY")
	if apiKey == "" {
		log.Fatal("Env: apiKey must be set")
	}

	myClient := &http.Client{Timeout: 10 * time.Second}
	newsapi := news.NewClient(myClient, apiKey, 20)

	searchCache := &sync.Map{}

	mux := http.NewServeMux()

	fs := http.FileServer(http.Dir("assets"))
	mux.Handle("/assets/", http.StripPrefix("/assets/", fs))

	mux.HandleFunc("/search", searchHandler(newsapi, searchCache))
	mux.HandleFunc("/", indexHandler)

	http.ListenAndServe(":"+port, mux)
}
