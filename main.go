package main

import (
	"github.com/lucas-clemente/quic-go/http3"
	"crypto/tls"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"time"
	"strings"
)
/*
Instructions for setting up this server:

- Install Go
- Install mkcert (https://github.com/FiloSottile/mkcert#installation)
- $ mkcert -install iffff you want the root CA to be installed automatically
-    (which makes things easier for Chrome!
-    the alternative is using ignore-certificate-errors-spki-list, Google it for QUIC)
-    otherwise, find rootCA.pem using "$ mkcert -CAROOT"
-    Firefox: Settings > certificates > Authorities > scroll down
-    import "rootCA.pem" > Trust for websites
-    (Or, just ignore the warning)
-    Chrome: use mkcert -install to install to system and chrome will use automatically
-    accept it. Then you might want to run with --origin-to-force-quic-on=localhost:1441
-    for H/3. Use inspect element > security to see the encryption suite used if you wish,
-    typically ChaCha for H/3.
-
- copy this file into a new folder, name it "main.go".
- $ mkcert localhost 127.0.0.1 ::1 <other domains/ips that can be used by the server>
- (Note: I haven't tested this tool using IPv6)
- Edit this "main.go" and set the constants below
- $ go run .

To generate a random file:
dd if=/dev/urandom of=randbytes20M.txt bs=20M count=1
*/

// The IP, port are for the server's device
// Keep the ':' for the port
const IP = "localhost"
const PORT = ":1441"

const CERT = "./localhost+2.pem"
const KEY = "./localhost+2-key.pem"
// Possible values: 1 (for 1.1), 2, 3
const HTTP = 3

// 12 is common between H/1.1 and H/2, 13 is common between H/2 and H/3
// Unfortunately TLS 1.3 cannot use the same ciphersuite as TLS 1.2
// due to Golang standard library restrictions.
// However, to check if crypto is the bottleneck you may try downloading
// and see if progresses with the exact same speed or much faster.
// The latter means that crypto is not the bottleneck.
const TLS_VERSION = 13

//https://stackoverflow.com/a/40699578
func ReceiveFile(w http.ResponseWriter, r *http.Request) int {
	r.ParseMultipartForm(1 << 60)
	file, _, err := r.FormFile("myFile")
	if err != nil {
		fmt.Println(err)
		return 0
	}
	defer file.Close()
	n, err := io.Copy(ioutil.Discard, file)
	if err != nil {
		fmt.Println(err)
	}
	return int(n)
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/upload.html", func(w http.ResponseWriter, req *http.Request) {
		if req.Method == "GET" {
			http.Redirect(w, req, "https://"+req.Host, http.StatusSeeOther)
		}
		w.Write([]byte(
			`<!DOCTYPE html>
      <html lang="en">
      <head>
        <meta charset="UTF-8" />
        <meta name="viewport" content="width=device-width, initial-scale=1.0" />
        <title>Document</title>
      </head>
      <body>`))
		w.Write([]byte(req.Proto + "</br>"))
		t1 := time.Now()
		size := ReceiveFile(w, req)
		t2 := time.Since(t1)
		x := fmt.Sprintf("%v", t2)
		if req.TLS.CipherSuite != 0 /*quic-go bug! lucas-clemente/quic-go/issues/3625 */ {
			w.Write([]byte(fmt.Sprintf("cipher ") + fmt.Sprintf("0x%.4x</br>", req.TLS.CipherSuite)))
		}
		w.Write([]byte(fmt.Sprintf("%v MB ", size/1000000) + fmt.Sprintf("%s</br>", x)))
		w.Write([]byte(fmt.Sprintf("%.3f MB/s ", float64(size)/float64(t2.Nanoseconds())*1000)))
		// Prefer to print to console only when the client downloads
		/*if size != 0 {
			fmt.Printf("h/%v %.3f MB/s ", req.ProtoMajor, float64(size)/float64(t2.Nanoseconds())*1000)
		}*/
		w.Write([]byte(`</body></html>`))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/" {
			i := 0
			str := strings.ReplaceAll(strings.ReplaceAll(req.URL.Path[1:], "G", "M000"), "M", "000000")
			_, err := fmt.Sscanf(str, "%d", &i)
			if (err != nil) || (len(str) == 0)  {
				http.Redirect(w, req, "https://"+req.Host, http.StatusSeeOther)
				// fmt.Print(err)
			}

			// Send file

			// https://golangbyexample.com/image-http-response-golang/
			w.Header().Set("Content-Type", "application/octet-stream")
			b := make([]byte, i)
			t1 := time.Now()
			w.Write(b)
			t2 := time.Since(t1)
			if i != 0 {
				fmt.Printf("h/%v %.3f MB/s ", req.ProtoMajor, float64(i)/float64(t2.Nanoseconds())*1000)
			}
		}
		w.Write([]byte(
			`<!DOCTYPE html>
      <html lang="en">
      <!--https://tutorialedge.net/golang/go-file-upload-tutorial/-->
      <head>
        <meta charset="UTF-8" />
        <meta name="viewport" content="width=device-width, initial-scale=1.0" />
        <title>Document</title>
      </head>
      <body>
        <form enctype="multipart/form-data" action="https://` +
				req.Host +
				`/upload.html" method="post">
          <input type="file" name="myFile" />
          <input type="submit" value="upload" />
        </form>
				<br>
				<a onclick="fetch('20M').then((response) => document.body.innerHTML+='<br>started; check Go console soon')">Download 20M file to browser memory</a><br>
				<a onclick="fetch('1G').then((response) => document.body.innerHTML+='<br>started; check Go console soon')">Download 1G file to browser memory</a><br>
				Manual: visit ./XM or ./XG to download files this big. Buttons above are preferred, to download to RAM rather than the disk.<br>
				Speed will appear in the Golang console.
      </body>
      </html>
      `,
		))
	})

	cfg := &tls.Config{
		MinVersion:               tls.VersionTLS12,
		MaxVersion:               tls.VersionTLS12,
	}

	if TLS_VERSION == 13 {
		cfg.MinVersion = tls.VersionTLS13
		cfg.MaxVersion = tls.VersionTLS13
	}

	srv := &http.Server{
		Addr:      PORT,
		Handler:   mux,
		TLSConfig: cfg,
	}

	if HTTP == 1 {
		srv.TLSNextProto = make(map[string]func(*http.Server, *tls.Conn, http.Handler))
	}

	if HTTP == 3 {
		// https://stackoverflow.com/q/70961167
		log.Fatal(http3.ListenAndServe(
				PORT,
				CERT,
				KEY,
				mux,
		))
	} else {
		log.Fatal(srv.ListenAndServeTLS(
			CERT,
			KEY,
		))
	}
}
