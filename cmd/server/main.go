package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strconv"

	"github.com/chocolatkey/clipremote"
	"github.com/chocolatkey/clipremote/pkg/commands"
	"github.com/chocolatkey/clipremote/pkg/packets"
	"github.com/chocolatkey/clipremote/pkg/preview"
	"github.com/sirupsen/logrus"
	"golang.org/x/image/bmp"
)

func main() {
	if len(os.Args) == 1 {
		println("Usage: server <Share URL>")
		return
	}

	// logrus.SetLevel(logrus.DebugLevel)

	ipAddresses, port, password, generation, err := clipremote.DecodeConfig(os.Args[1])
	logrus.Infoln("share generation", generation)

	// Connect
	client, err := clipremote.Connect(
		ipAddresses,
		port,
		generation,
	)
	if err != nil {
		println("Failed connecting to CSP instance")
		panic(err)
	}

	// Auth
	client.Authenticate(func(scp *packets.ServerCommand, err error) {
		if err != nil {
			if scp != nil {
				bin, _ := json.Marshal(scp)
				println(string(bin))
			}
			panic(err)
		}

		println("Client authenticated")
	}, password)

	http.HandleFunc("/request", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Bad request body", http.StatusBadRequest)
			return
		}
		if !client.Alive() {
			http.Error(w, "Not ready", http.StatusServiceUnavailable)
			return
		}

		// Form data
		command := r.Form.Get("command")
		detail := r.Form.Get("detail")
		if command == "" {
			http.Error(w, "Missing command", http.StatusBadRequest)
			return
		}

		var detailData interface{}
		if len(detail) > 2 {
			var detailDataArray []interface{}
			if err = json.Unmarshal([]byte(detail), &detailDataArray); err != nil {
				detailDataMap := make(map[string]interface{})
				if err = json.Unmarshal([]byte(detail), &detailDataMap); err != nil {
					http.Error(w, "Invalid data, must be valid JSON if included", http.StatusBadRequest)
					return
				} else {
					detailData = detailDataMap
				}
			} else {
				detailData = detailDataArray
			}
		}

		scp, err := client.SendCommandSync(commands.Command(command), detailData)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("content-type", "application/json; charset=utf-8")
		json.NewEncoder(w).Encode(scp)
	})

	http.HandleFunc("/preview", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Bad request body", http.StatusBadRequest)
			return
		}

		toUint := func(s string) (uint, error) {
			if s == "" {
				return 0, errors.New("empty")
			}
			num, err := strconv.ParseUint(s, 10, 64)
			if err != nil {
				return 0, err
			}
			return uint(num), nil
		}

		blockIndex, err := toUint(r.FormValue("block_index"))
		if err != nil {
			http.Error(w, "Invalid/empty block_index", http.StatusBadRequest)
			return
		}

		blockBottom, err := toUint(r.FormValue("block_bottom"))
		if err != nil {
			http.Error(w, "Invalid/empty block_bottom", http.StatusBadRequest)
			return
		}

		blockRight, err := toUint(r.FormValue("block_right"))
		if err != nil {
			http.Error(w, "Invalid/empty block_right", http.StatusBadRequest)
			return
		}

		blockTop, err := toUint(r.FormValue("block_top"))
		if err != nil {
			http.Error(w, "Invalid/empty block_top", http.StatusBadRequest)
			return
		}

		blockLeft, err := toUint(r.FormValue("block_left"))
		if err != nil {
			http.Error(w, "Invalid/empty block_left", http.StatusBadRequest)
			return
		}

		canvasIndex, err := toUint(r.FormValue("canvas_index"))
		if err != nil {
			http.Error(w, "Invalid/empty canvas_index", http.StatusBadRequest)
			return
		}

		galleryIdentificationNumber, err := toUint(r.FormValue("gallery_identification_number"))
		if err != nil {
			http.Error(w, "Invalid/empty gallery_identification_number", http.StatusBadRequest)
			return
		}

		scp, err := client.SendCommandSync(
			commands.PreviewWebtoonFromClient,
			commands.DetailPreviewWebtoonFromClientReadPreviewBlock{
				Operation:                   "ReadPreviewBlock",
				BlockIndex:                  blockIndex,
				BlockBottom:                 blockBottom,
				BlockRight:                  blockRight,
				BlockTop:                    blockTop,
				BlockLeft:                   blockLeft,
				CanvasIndex:                 canvasIndex,
				GalleryIdentificationNumber: galleryIdentificationNumber,
			},
		)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if len(scp.Data) == 0 {
			http.Error(w, "No image data", http.StatusInternalServerError)
			return
		}

		rgbData, err := base64.RawStdEncoding.DecodeString(string(scp.Data))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Render preview as PNG
		img := preview.Decode(rgbData, int(blockRight-blockLeft), int(blockBottom-blockTop))
		w.Header().Set("content-type", "image/bmp")
		w.WriteHeader(http.StatusOK)
		bmp.Encode(w, img)
	})

	http.ListenAndServe(":8089", nil)
}
