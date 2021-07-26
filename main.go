package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

const (
	host        = "https://getpocket.com/v3"
	retrieveUrl = host + "/get"
)

var (
	token = os.Getenv("POCKET_ACCESS_TOKEN")
	key   = os.Getenv("POCKET_CONSUMER_KEY")
)

var (
	generateScreenshots = true
	outputLogs          = true
)

// Struct for Results of our main retrieve query.
type Result struct {
	List     map[string]ResultItem
	Status   int
	Complete int
	Since    int
}

// Struct for each item of our main retrieve query.
type ResultItem struct {
	ItemID        int    `json:"item_id,string"`
	ResolvedID    int    `json:"resolved_id,string"`
	GivenURL      string `json:"given_url"`
	ResolvedURL   string `json:"resolved_url"`
	GivenTitle    string `json:"given_title"`
	ResolvedTitle string `json:"resolved_title"`
	Favorite      int    `json:"favorite,string"`
	Status        int    `json:"status,string"`
	Excerpt       string `json:"excerpt"`
	IsArticle     int    `json:"is_article,string"`
	HasImage      int    `json:"has_image,string"`
	HasVideo      int    `json:"has_video,string"`
	WordCount     int    `json:"word_count,string"`
	TopImageURL   string `json:"top_image_url"`
	Tags          map[string]map[string]interface{}
	Authors       map[string]map[string]interface{}
	Images        map[string]map[string]interface{}
	Videos        map[string]map[string]interface{}
	SortID        int `json:"sort_id"`
}

// Struct for a single item in our response.
type Item struct {
	ID      int    `json:"item_id"`
	Title   string `json:"title"`
	URL     string `json:"url"`
	Excerpt string `json:"excerpt"`
	Type    string `json:"type"`
	SortID  int    `json:"sort_id"`
	Image   string `json:"image"`
}

// Get all items from the Pocket API.
func retrievePocketItems() Result {
	// Set up an HTTP client.
	client := &http.Client{}

	// Start our request to the retrieve endpoint.
	req, err := http.NewRequest("GET", retrieveUrl, nil)
	if err != nil {
		log.Fatalf("Error building request  %s\n", req.URL)
		log.Fatal(err)
	}

	// Build up our query args for the request, including our key/token for access.
	q := req.URL.Query()
	q.Add("consumer_key", key)
	q.Add("access_token", token)
	q.Add("detailType", "complete")
	q.Add("state", "unread")
	q.Add("sort", "newest")
	req.URL.RawQuery = q.Encode()

	if outputLogs {
		fmt.Printf("Retrieving %s\n", req.URL)
	}

	// Perform the request.
	resp, err := client.Do(req)
	if err != nil {
		log.Fatalf("Error getting request  %s\n", req.URL)
		log.Fatal(err)
	}
	defer resp.Body.Close()

	// Make sure we get a valid response.
	if resp.StatusCode != 200 {
		log.Fatalf("Did not get 200 for request  %s\n", req.URL)
		log.Fatal(err)
	}

	// Read the response of our request.
	bodyBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatalf("Error reading body  %s\n", req.URL)
		log.Fatal(err)
	}

	// Unmarshal our response and pass it on.
	var results Result
	json.Unmarshal(bodyBytes, &results)
	return results
}

// Check if a file exists locally.
func fileExists(filename string) bool {
	if _, err := os.Stat(filename); err != nil {
		if os.IsNotExist(err) {
			return false
		}
	}
	return true
}

// Trigger a headless Chrome request to take a screenshot.
func chromeTakeScreenshot(url string, imageBuf *[]byte) chromedp.Tasks {
	return chromedp.Tasks{
		chromedp.Navigate(url),
		chromedp.ActionFunc(func(ctx context.Context) (err error) {
			*imageBuf, err = page.CaptureScreenshot().WithQuality(95).Do(ctx)
			return err
		}),
	}
}

// Save a screenshot for a url.
func saveScreenshot(url string, filename string) bool {
	if outputLogs {
		fmt.Printf("Saving screenshot (%s) for %s\n", filename, url)
	}

	// Start an instance of Chrome.
	ctx, cancel := chromedp.NewContext(context.Background())
	defer cancel()

	// Start an image buffer and take a screenshot.
	var imageBuf []byte
	if err := chromedp.Run(ctx, chromeTakeScreenshot(url, &imageBuf)); err != nil {
		if outputLogs {
			fmt.Printf("Could not take screenshot for %s \n", filename)
		}
		return false
	}

	// Write our image to the local filesystem.
	if err := ioutil.WriteFile(filename, imageBuf, 0o644); err != nil {
		if outputLogs {
			fmt.Printf("Could not write screenshot for %s \n", filename)
		}
		return false
	}

	return true
}

// Save a remote image to our local filesystem.
func saveRemoteImage(src, filename string) bool {
	if outputLogs {
		fmt.Printf("Saving image (%s) for %s\n", filename, src)
	}
	// Start our http request.
	resp, err := http.Get(src)
	if err != nil {
		if outputLogs {
			fmt.Printf("Request failed for %s \n", src)
		}
		return false
	}
	defer resp.Body.Close()

	// Make sure we got a valid response.
	if resp.StatusCode != 200 {
		if outputLogs {
			fmt.Printf("Did not get 200 status for request %s \n", src)
		}
		return false
	}

	// Create a file on our filesystem.
	file, err := os.Create(filename)
	if err != nil {
		if outputLogs {
			fmt.Printf("Could not create file %s \n", filename)
		}
		return false
	}
	defer file.Close()

	// Write the remote image to our local file.
	_, err = io.Copy(file, resp.Body)
	if err != nil {
		if outputLogs {
			fmt.Printf("Could not write file %s \n", filename)
		}
		return false
	}

	return true
}

// Save either a remote image or screenshot for an item
func saveImageForItem(item ResultItem, filename string) bool {
	// If the item does have an image, attempt to process it.
	if item.HasImage != 0 {
		// Loop through all attached images and grab the source, width, and height.
		for _, v := range item.Images {
			source := v["src"].(string)

			// Grab the youtube thumbnail if it is in the images list.
			// @todo add more of these?
			if strings.Contains(source, "i.ytimg.com") || strings.Contains(source, "img.youtube.com") {
				return saveRemoteImage(source, filename)
			}
		}
	} else if item.HasImage == 2 {
		// If the item itself is an image, save it.
		return saveRemoteImage(item.ResolvedURL, filename)
	}

	// If we didn't save an image, then save the screenshot of it.
	return saveScreenshot(item.ResolvedURL, filename)
}

// Get and process all our items from Pocket.
func pocketItems() []Item {
	// Retrieve a list of items from the API.
	results := retrievePocketItems()

	// Set up a slice to contain all our processed items.
	items := []Item{}

	// Start looping through our items to process them.
	for _, item := range results.List {
		// Use the resolved title, but fallback to the given.
		title := item.ResolvedTitle
		if item.ResolvedTitle == "" {
			title = item.GivenTitle
		}

		// Make sure we have a url.
		url := item.ResolvedURL
		if url == "" {
			url = item.GivenURL

			// Also set the resolved url so we don't need to check again.
			item.ResolvedURL = url
		}

		if outputLogs {
			fmt.Printf("Processing %s (%s) \n", title, url)
		}

		// For the pocket api, hasImage/hasVideo gets set as 2 if that is the content type.
		contentType := "article"
		if item.HasImage == 2 {
			contentType = "image"
		} else if item.HasVideo == 2 {
			contentType = "video"
		}

		// Save our screenshots & images in our images dir, with the ID as the filename.
		filename := fmt.Sprintf("images/%d.png", item.ItemID)

		// Check to see if we have a file for the image.
		imageSaved := fileExists(filename)

		// If screenshot generation is enabled, check to see if we can save the image.
		if generateScreenshots && !imageSaved {
			imageSaved = saveImageForItem(item, filename)
		}
		// Only set the filename if the image is saved.
		publicFilename := ""
		if imageSaved {
			publicFilename = fmt.Sprintf("http://localhost:4000/%s", filename)
		}

		i := Item{
			ID:      item.ItemID,
			Title:   title,
			URL:     url,
			Excerpt: item.Excerpt,
			Type:    contentType,
			SortID:  item.SortID,
			Image:   publicFilename,
		}

		items = append(items, i)
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].SortID < items[j].SortID
	})

	return items
}

func serve() {
	// Handle the base url to return our JSON output
	// http.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
	// 	// Toggle our screenshot generation flag to false,
	// 	// as we don't want to slow the response down.
	// 	generateScreenshots = false

	// 	// Get our pocket itet and process all our items from Pocket.ems.
	// 	items := pocketItems()
	// 	// Retrieve a list of items from the API.

	// 	// Create our JSON output.
	// 	// Start looping through our items to process them.
	// 	output, err := json.Marshal(items)
	// 	if err != nil {
	// 		log.Fatal("Failed marshal JSON")
	// 		log.Fatal(err)
	// 	}

	// 	// Send back our headers.
	// 	w.Header().Set("Content-Type", "application/json")
	// 	w.WriteHeader(http.StatusCreated)

	// 	// Send our JSON.
	// 	fmt.Fprint(w, string(output))
	// })

	http.Handle("/", http.FileServer(http.Dir("./cache")))

	// The screenshots folder will serve our static folder of images.
	http.Handle("/images/", http.StripPrefix("/images/", http.FileServer(http.Dir("./images"))))

	// Serve it on port 4000.
	fmt.Println("Starting server at http://localhost:4000")
	if err := http.ListenAndServe("localhost:4000", nil); err != nil {
		log.Fatal(err)
	}
}

func get() {
	items := pocketItems()
	f, err := os.Create("cache/all.json")
	if err != nil {
		log.Fatal("Failed creating cache file")
		log.Fatal(err)
	}
	defer f.Close()

	data, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		log.Fatal("Failed marshaling JSON")
		log.Fatal(err)
	}

	_, err = f.WriteString(string(data))
	if err != nil {
		log.Fatal("Failed writing cache file")
		log.Fatal(err)
	}
}

func main() {
	// If we call it with the argument "get", then we want to just
	// get all the items, otherwise we're going to be a webserver.
	if len(os.Args) > 1 && os.Args[1] == "get" {
		get()
	} else {
		// outputLogs = false
		serve()
	}
}
