# Clip Studio Paint Remote

Communicate with CSP using their ["companion mode"](https://tips.clip-studio.com/en-us/articles/5995) protocol.
This is a WIP implementation, and help is appreciated documenting the various commands, requests and responses.

An example HTTP server with a simple but flexible API that lets you send commands and receive back JSON.
To get started:

1. Click on the "Connect to smartphone" icon in CSP. A QR code will be shown
2. Scan the QR code using a smartphone, or take a screenshot of it and paste into a website such as this to decode: https://qr-code-scanner.net/#paste
3. Get the resulting URL in the form `https://companion.clip-studio.com/rc/en-us?s=XXX` and run the server with it using `go run cmd/server/main.go "<URL>"`
4. Check the command output. If successful, an HTTP server will be started at `http://localhost:8089`
5. Run commands using a URL like this (query params or POST body) http://localhost:8089/request?command=GetModifyKeyString&detail={%22AltPushed%22:false,%22CtrlPushed%22:false,%22ShiftPushed%22:false}

More docs and tips coming later.