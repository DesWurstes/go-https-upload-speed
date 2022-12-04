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
)
/*
Instructions for setting up this server:

- Install Go
- Install mkcert (https://github.com/FiloSottile/mkcert#installation)
- cd into a new folder with the attached go file named "main.go".
- $ mkcert -install iffff you want the root CA to be installed automatically
-    otherwise, find rootCA.pem using "$ mkcert -CAROOT"
-    then go to Firefox Settings > certificates > Authorities > scroll down
-    import "rootCA.pem" > Trust for websites
- $ mkcert localhost 127.0.0.1 ::1 <other domains/ips that can be used by the server>
- (Note: I don't suggest using IPv6)
- Open this "main.go" and set the constants below
- $ go run .

Alternatively, without installing Root CA, by temporarily allowing it:
Connect to https://YOUR_IP:1441/ with Firefox.
Ignore the certificate warning.


Change `network.http.spdy.enabled.http2` to compare
Or alternatively the HTTP2 constant below.

To generate a random file:
dd if=/dev/urandom of=randbytes1G.txt bs=1G count=1
*/

// The IP, port are for the server's device
// Keep the ':' for the port
const IP = "localhost"
const PORT = ":1441"

const CERT = "./localhost+2.pem"
const KEY = "./localhost+2-key.pem"
const HTTP = 3
const ENFORCE_CIPHERSUITES = true

//https://stackoverflow.com/a/40699578
func receiveFile(w http.ResponseWriter, r *http.Request) int {
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

// TCP code could have been copied from here
// https://github.com/lucas-clemente/quic-go/blob/8d496ebb5e3574a687d8a3b99988a5a51184884b/http3/server.go#L212
// func ListenAndServe(addr, certFile, keyFile string,

// Returns possible http3 config
func startServer(vHTTP int, altSVCPresent bool, mux *http.ServeMux, cfg *tls.Config) *http3.Server {
	srv := &http3.Server{
		Addr:      PORT,
		Handler:   mux,
		// https://github.com/lucas-clemente/quic-go/blob/f2fa98c/interop/server/main.go
		TLSConfig: cfg,
	}

	if vHTTP != 3 {
		srv := &http.Server{
			Addr:      PORT,
			Handler:   mux,
			TLSConfig: cfg,
		}

		if vHTTP == 1 {
			// Alternative ways: https://stackoverflow.com/a/67775105
			srv.TLSNextProto = make(map[string]func(*http.Server, *tls.Conn, http.Handler))
		}

		log.Fatal(srv.ListenAndServeTLS(
			CERT,
			KEY,
		))
	}

	return srv
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/upload.html", func(w http.ResponseWriter, req *http.Request) {
		if req.Method == "GET" {
			http.Redirect(w, req, "https://"+IP+PORT, http.StatusSeeOther)
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
		size := receiveFile(w, req)
		t2 := time.Since(t1)
		x := fmt.Sprintf("%v", t2)
		w.Write([]byte(fmt.Sprintf("%v MB ", size/1000000) + fmt.Sprintf("%s</br>", x)))
		w.Write([]byte(fmt.Sprintf("%.3f MB/s ", float64(size)/float64(t2.Nanoseconds())*1000)))
		if size != 0 {
			fmt.Printf("h/%v %.3f MB/s ", req.ProtoMajor, float64(size)/float64(t2.Nanoseconds())*1000)
		}
		w.Write([]byte(`</body></html>`))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/" {
			http.Redirect(w, req, "https://"+IP+PORT, http.StatusSeeOther)
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
      </body>
      </html>
      `,
		))
	})

	cfg := &tls.Config{
		MinVersion:               tls.VersionTLS12,
		MaxVersion:               tls.VersionTLS12,
		PreferServerCipherSuites: true,
	}

	if ENFORCE_CIPHERSUITES {
		cfg.PreferServerCipherSuites = true
		// https://github.com/golang/go/issues/11047
		cfg.CipherSuites = []uint16{
    	tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
    	tls.TLS_RSA_WITH_AES_128_CBC_SHA,
    }
	}

	if HTTP == 3 {
		quicServer := startServer(3, false, mux, cfg)
		upgradeMux := http.NewServeMux()
		upgradeMux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			// https://github.com/lucas-clemente/quic-go/blob/8d496ebb5e3574a687d8a3b99988a5a51184884b/http3/server.go#L691
			quicServer.SetQuicHeaders(w.Header())
			mux.ServeHTTP(w, r)
		})
		go startServer(2, true, upgradeMux, cfg)
		log.Fatal(quicServer.ListenAndServeTLS(
				CERT,
				KEY,
			),
		)
		// This starts both servers but cannot be forced specific ciphers since higher-level
		/*log.Fatal(http3.ListenAndServe(
				PORT,
				CERT,
				KEY,
				mux,
			),
		)*/
	} else {
		startServer(HTTP, false, mux, cfg)
	}
}
