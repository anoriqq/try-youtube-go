package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-xray-sdk-go/xray"
	"github.com/gorilla/mux"
	"google.golang.org/api/option"
	googleHTTP "google.golang.org/api/transport/http"
	"google.golang.org/api/youtube/v3"
)

var (
	googleAPIKey *string
)

func init() {
	googleAPIKey = flag.String("apikey", "", "google cloud platform api credential")
}

type youtubeService struct {
	*youtube.Service
}

func (s youtubeService) ListVideos(ctx context.Context, videoID string) (*youtube.VideoListResponse, error) {
	return s.Videos.List([]string{"snippet"}).Id(videoID).Context(ctx).Do()
}

func NewYoutubeService() (*youtubeService, error) {
	ctx := context.Background()

	transport, err := googleHTTP.NewTransport(ctx, http.DefaultTransport, option.WithAPIKey(*googleAPIKey))
	if err != nil {
		return &youtubeService{}, err
	}

	c := &http.Client{
		Transport: transport,
		Timeout:   5 * time.Second,
	}

	c = xray.Client(c)

	s, err := youtube.NewService(ctx, option.WithHTTPClient(c))
	if err != nil {
		return &youtubeService{}, err
	}

	return &youtubeService{s}, nil
}

func main() {
	flag.Parse()

	s, err := NewYoutubeService()
	if err != nil {
		panic(err)
	}

	xraySegment := xray.NewDynamicSegmentNamer("myApp", "youtube.googleapis.com")

	r := mux.NewRouter()
	r.HandleFunc("/", xray.Handler(xraySegment, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})).ServeHTTP)
	r.HandleFunc("/500", xray.Handler(xraySegment, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal server error"))
	})).ServeHTTP)
	r.HandleFunc("/youtube-video/{video-id}", xray.Handler(xraySegment, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sub := strings.TrimPrefix(r.URL.Path, "/youtube-video")
		_, videoID := filepath.Split(sub)
		res, err := s.ListVideos(r.Context(), videoID)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("internal server error"))
			log.Fatal(err)
			return
		}

		if len(res.Items) < 1 {
			m := "message not found"
			log.Fatal(m)
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(m))
			return
		}

		v := res.Items[0]
		videoTitle := v.Snippet.Title

		log.Default().Println(videoTitle)

		w.Write([]byte(videoTitle))
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP)

	log.Fatal(http.ListenAndServe(":8000", r))
}
