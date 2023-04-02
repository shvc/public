// $ go run _examples/main.go

package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	mrand "math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/elastic/go-elasticsearch/v7"
	"github.com/elastic/go-elasticsearch/v7/esapi"
)

func init() {
	mrand.Seed(time.Now().UnixNano())
}

var (
	es         *elasticsearch.Client
	server         = "http://192.168.56.2:9200"
	userName       = "elastic"
	password       = "ChangeMe"
	index          = "test"
	outputFile     = filepath.Join(os.TempDir(), "es-out.log")
	number     int = 1
	scrollSize int = 100
	debug          = false
)

var (
	outputFd *os.File
)

type Result struct {
	ScrollID string `json:"_scroll_id"`
	Took     int    `json:"took"`
	TimedOut bool   `json:"timed_out"`
	Hits     struct {
		Total struct {
			Value    int    `json:"value"`
			Relation string `json:"relation"`
		} `json:"total"`
		Hits []struct {
			ID     string          `json:"_id"`
			Index  string          `json:"_index"`
			Source json.RawMessage `json:"_source"`
		} `json:"hits"`
	} `json:"hits"`
	Aggregations map[string]interface{} `json:"aggregations"`
}

func RandomString(len int) string {
	buf := make([]byte, len)
	_, err := rand.Read(buf)
	if err != nil {
		for i := 0; i < len; i++ {
			buf[i] = byte(mrand.Intn(128))
		}
	}
	return hex.EncodeToString(buf)
}

func logFatal(r io.Reader, status string) {
	var e map[string]interface{}
	if err := json.NewDecoder(r).Decode(&e); err != nil {
		log.Fatalf("Error parsing the response body: %s", err)
	} else {
		// Print the response status and error information.
		log.Fatalf("[%s] %s: %s",
			status,
			e["error"].(map[string]interface{})["type"],
			e["error"].(map[string]interface{})["reason"],
		)
	}
}

func main() {
	flag.StringVar(&server, "s", server, "server address")
	flag.StringVar(&userName, "u", userName, "user name")
	flag.StringVar(&password, "p", password, "user password")
	flag.StringVar(&index, "i", index, "index name")
	flag.StringVar(&outputFile, "o", outputFile, "output file name")
	flag.IntVar(&number, "n", number, "insert document number")
	flag.IntVar(&scrollSize, "size", scrollSize, "scroll size")

	flag.BoolVar(&debug, "debug", debug, "debug")
	flag.Parse()
	log.SetFlags(0)

	var r map[string]interface{}

	// Initialize a client with the default settings.
	cfg := elasticsearch.Config{
		Addresses: strings.Split(server, ","),
		Username:  userName,
		Password:  password,
	}
	var err error
	es, err = elasticsearch.NewClient(cfg)
	if err != nil {
		log.Fatalf("Error creating the client: %s", err)
	}

	// 1. Get cluster info
	res, err := es.Info()
	if err != nil {
		log.Fatalf("Error getting response: %s", err)
	}
	defer res.Body.Close()
	// Check response status
	if res.IsError() {
		log.Fatalf("Error: %s", res.String())
	}
	// Deserialize the response into a map.
	if err := json.NewDecoder(res.Body).Decode(&r); err != nil {
		log.Fatalf("Error parsing the response body: %s", err)
	}
	// Print client and server version numbers.
	log.Printf("Client: %s", elasticsearch.Version)
	log.Printf("Server: %s", r["version"].(map[string]interface{})["number"])
	log.Println(strings.Repeat("~", 37))

	// 2. Index documents
	title := RandomString(8)
	for i := 0; i < int(number); i++ {
		// Build the request body.
		data, err := json.Marshal(struct {
			Title string `json:"title"`
			Age   int    `json:"age"`
		}{
			Title: fmt.Sprintf("%s-%v", title, i),
			Age:   i + 10,
		})
		if err != nil {
			log.Fatalf("Error marshaling document: %s", err)
		}

		// Set up the request object.
		req := esapi.IndexRequest{
			Index: index,
			//DocumentID: strconv.Itoa(i + 1),
			Body:    bytes.NewReader(data),
			Refresh: "true",
		}

		// Perform the request with the client.
		res, err := req.Do(context.Background(), es)
		if err != nil {
			log.Fatalf("Error getting response: %s", err)
		}
		defer res.Body.Close()

		if res.IsError() {
			log.Printf("[%s] Error indexing document ID=%d", res.Status(), i+1)
		} else {
			// Deserialize the response into a map.
			var r map[string]interface{}
			if err := json.NewDecoder(res.Body).Decode(&r); err != nil {
				log.Printf("Error parsing the response body: %s", err)
			} else {
				// Print the response status and indexed document version.
				log.Printf("[%s] %s; version=%d", res.Status(), r["result"], int(r["_version"].(float64)))
			}
		}

	}

	log.Println(strings.Repeat("-", 37))

	// 3. Search for the indexed documents

	outputFd, err = os.OpenFile(outputFile, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0666)
	if err != nil {
		log.Fatal(err)
	}
	//normalSearch(title)

	scrollSearch(title)
}

func scrollSearch(title string) error {
	var r = Result{}
	var buf bytes.Buffer
	query := map[string]interface{}{
		"query": map[string]interface{}{
			"match": map[string]interface{}{
				"title": title,
			},
		},
	}
	if err := json.NewEncoder(&buf).Encode(query); err != nil {
		log.Fatalf("Error encoding query: %s", err)
	}
	res, err := es.Search(
		es.Search.WithIndex(index),
		es.Search.WithBody(&buf),
		es.Search.WithSize(scrollSize),
		es.Search.WithScroll(time.Minute),
		//es.Search.WithPretty(),
	)
	if err != nil {
		return err
	}
	if res.IsError() {
		return errors.New("an error")
	}
	defer res.Body.Close()
	if debug {
		// io.Copy(os.Stdout, res.Body)
		// return nil
		/*
			{
			  "_scroll_id" : "FGluY2x1ZGVfY29udGV4dF91dWlkDXF1ZXJ5QW5kRmV0Y2gBFmFwNmVsWm9OU2RXTjI0RlA1RWRIVUEAAAAAAAAAOxY5UDgtaDVyN1RFYW5QTm5NY0ZwWEJB",
			  "took" : 2,
			  "timed_out" : false,
			  "_shards" : {
			    "total" : 1,
			    "successful" : 1,
			    "skipped" : 0,
			    "failed" : 0
			  },
			  "hits" : {
			    "total" : {
			      "value" : 9,
			      "relation" : "eq"
			    },
			    "max_score" : 1.6805339,
			    "hits" : [
			      {
			        "_index" : "test",
			        "_type" : "_doc",
			        "_id" : "uwDKQocBMP718tB8p5r1",
			        "_score" : 1.6805339,
			        "_source" : {
			          "title" : "b5054aa59b17a5d5-0",
			          "age" : 10
			        }
			      },
			      {
			        "_index" : "test",
			        "_type" : "_doc",
			        "_id" : "vADKQocBMP718tB8qJoH",
			        "_score" : 1.6805339,
			        "_source" : {
			          "title" : "b5054aa59b17a5d5-1",
			          "age" : 11
			        }
			      }
			    ]
			  }
			}
		*/

	}
	if err := json.NewDecoder(res.Body).Decode(&r); err != nil {
		return err
	}

	log.Printf("total:%v, size:%v, took:%v", r.Hits.Total.Value, scrollSize, r.Took)
	if err = outputToFile(&r); err != nil {
		return err
	}

	scrollId := r.ScrollID
	if scrollId == "" {
		return errors.New("no scroll id")
	}

	batchNum := 0
	for {
		batchNum++
		res, err := es.Scroll(
			es.Scroll.WithScrollID(scrollId),
			es.Scroll.WithScroll(time.Minute),
			//es.Scroll.WithPretty(),
		)
		if err != nil {
			log.Fatalf("Error: %s", err)
			return err
		}
		if res.IsError() {
			logFatal(res.Body, res.Status())
		}
		if debug {
			//io.Copy(os.Stdout, res.Body)
			/*
				{
				  "_scroll_id" : "FGluY2x1ZGVfY29udGV4dF91dWlkDXF1ZXJ5QW5kRmV0Y2gBFmFwNmVsWm9OU2RXTjI0RlA1RWRIVUEAAAAAAAAAPxY5UDgtaDVyN1RFYW5QTm5NY0ZwWEJB",
				  "took" : 1,
				  "timed_out" : false,
				  "_shards" : {
				    "total" : 1,
				    "successful" : 1,
				    "skipped" : 0,
				    "failed" : 0
				  },
				  "hits" : {
				    "total" : {
				      "value" : 9,
				      "relation" : "eq"
				    },
				    "max_score" : 2.118156,
				    "hits" : [
				      {
				        "_index" : "test",
				        "_type" : "_doc",
				        "_id" : "2wDbQocBMP718tB8qppM",
				        "_score" : 2.118156,
				        "_source" : {
				          "title" : "b5ba7339fedf0486-4",
				          "age" : 14
				        }
				      },
				      {
				        "_index" : "test",
				        "_type" : "_doc",
				        "_id" : "3ADbQocBMP718tB8qppg",
				        "_score" : 2.118156,
				        "_source" : {
				          "title" : "b5ba7339fedf0486-5",
				          "age" : 15
				        }
				      }
				    ]
				  }
				}
			*/
		}

		scrollResult := Result{}
		if err := json.NewDecoder(res.Body).Decode(&scrollResult); err != nil {
			res.Body.Close()
			return err
		}
		res.Body.Close()

		log.Printf("batch:%v, took:%v", batchNum, scrollResult.Took)
		// hits array empty means finished
		lenOfNextResult := len(scrollResult.Hits.Hits)
		if lenOfNextResult == 0 {
			break
		}
		scrollId = scrollResult.ScrollID
		if err = outputToFile(&scrollResult); err != nil {
			return err
		}
	}

	return nil
}

func outputToFile(r *Result) (err error) {
	for _, v := range r.Hits.Hits {
		if _, err = outputFd.Write(v.Source); err != nil {
			return err
		}
		outputFd.Write([]byte("\n"))
	}
	return
}

func normalSearch(title string) {
	var r map[string]interface{}
	var buf bytes.Buffer
	query := map[string]interface{}{
		"query": map[string]interface{}{
			"match": map[string]interface{}{
				"title": title,
			},
		},
	}
	if err := json.NewEncoder(&buf).Encode(query); err != nil {
		log.Fatalf("Error encoding query: %s", err)
	}

	// Perform the search request.
	res, err := es.Search(
		es.Search.WithContext(context.Background()),
		es.Search.WithIndex(index),
		es.Search.WithBody(&buf),
		es.Search.WithTrackTotalHits(true),
		//es.Search.WithPretty(),
	)
	if err != nil {
		log.Fatalf("Error getting response: %s", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		logFatal(res.Body, res.Status())
	}

	if debug {
		/*
			{
			  "took" : 1,
			  "timed_out" : false,
			  "_shards" : {
			    "total" : 1,
			    "successful" : 1,
			    "skipped" : 0,
			    "failed" : 0
			  },
			  "hits" : {
			    "total" : {
			      "value" : 2,
			      "relation" : "eq"
			    },
			    "max_score" : 2.4159138,
			    "hits" : [
			      {
			        "_index" : "test",
			        "_type" : "_doc",
			        "_id" : "qwDEQocBMP718tB8y5rl",
			        "_score" : 2.4159138,
			        "_source" : {
			          "title" : "beb19a9b9a396517-0",
			          "age" : 10
			        }
			      },
			      {
			        "_index" : "test",
			        "_type" : "_doc",
			        "_id" : "rADEQocBMP718tB8y5r1",
			        "_score" : 2.4159138,
			        "_source" : {
			          "title" : "beb19a9b9a396517-1",
			          "age" : 11
			        }
			      }
			    ]
			  }
			}
		*/
		io.Copy(os.Stdout, res.Body)
		return
	}

	if err := json.NewDecoder(res.Body).Decode(&r); err != nil {
		log.Fatalf("Error parsing the response body: %s", err)
	}

	// Print the response status, number of results, and request duration.
	log.Printf(
		"[%s] %d hits; took: %dms",
		res.Status(),
		int(r["hits"].(map[string]interface{})["total"].(map[string]interface{})["value"].(float64)),
		int(r["took"].(float64)),
	)
	// Print the ID and document source for each hit.
	for _, hit := range r["hits"].(map[string]interface{})["hits"].([]interface{}) {
		log.Printf(" * ID=%s, %s", hit.(map[string]interface{})["_id"], hit.(map[string]interface{})["_source"])
	}

	log.Println(strings.Repeat("=", 37))
}
