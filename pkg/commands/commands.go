package commands

type Command string

const (
	TellHeartbeat            Command = "TellHeartbeat"            // Send heartbeat to server for keepalive
	Authenticate             Command = "Authenticate"             // Authenticate with the server
	GetModifyKeyString       Command = "GetModifyKeyString"       //  Get/Set pressed modifier keys (Ctrl, Alt, Shift)
	GetServerSelectedTabKind Command = "GetServerSelectedTabKind" // Get selected tab from server
	SetServerSelectedTabKind Command = "SetServerSelectedTabKind" // When tab in remote control app is selected
	PreviewWebtoonFromClient Command = "PreviewWebtoonFromClient" // Preview webtoon from remote control app
)

// CommandGetModifyKeyString //

type DetailGetModifyKeyStringRequest struct {
	AltPushed   bool
	CtrlPushed  bool
	ShiftPushed bool
}

type DetailGetModifyKeyStringResponse struct {
	AltDescription   string
	CtrlDescription  string
	ShiftDescription string
	SystemKind       string // E.g. "Windows"
}

// CommandGetServerSelectedTabKind //

type DetailGetServerSelectedTabKindResponse struct {
	ServerSelectedTabKind string // Typically "Invalid"
}

// CommandPreviewWebtoonFromClient //

type DetailPreviewWebtoonFromClientRequestUpdateGallery struct {
	MaxLength uint
	Operation string // "UpdateGallery"
}

type DetailPreviewWebtoonFromServerResponse struct {
	Operation   string // "ResetCanvas"
	CanvasIndex uint
}

/*
Example:

	{
		"Operation": "UpdateGallery",
		"GalleryIdentificationNumber": 1,
		"CanvasSizeArray": [{
			"CanvasHeight": 22153,
			"CanvasWidth": 690
		}, {
			"CanvasHeight": 10406,
			"CanvasWidth": 345
		}, {
			"CanvasHeight": 10561,
			"CanvasWidth": 345
		}, {
			"CanvasHeight": 10612,
			"CanvasWidth": 345
		}, {
			"CanvasHeight": 10140,
			"CanvasWidth": 345
		}, {
			"CanvasHeight": 10617,
			"CanvasWidth": 345
		}, {
			"CanvasHeight": 4660,
			"CanvasWidth": 345
		}],
		"CanvasCount": 7
	}
*/
type DetailPreviewWebtoonFromClientResponseUpdateGallery struct {
	Operation                   string // "UpdateGallery"
	GalleryIdentificationNumber uint
	CanvasSizeArray             []struct {
		CanvasHeight uint
		CanvasWidth  uint
	}
	CanvasCount uint
}

/*
Example:

	{
		"BlockIndex": 0,
		"BlockBottom": 1024,
		"BlockRight": 690,
		"BlockTop": 0,
		"BlockLeft": 0,
		"CanvasIndex": 0,
		"GalleryIdentificationNumber": 1,
		"Operation": "ReadPreviewBlock"
	}
*/
type DetailPreviewWebtoonFromClientReadPreviewBlock struct {
	Operation                   string // "ReadPreviewBlock"
	BlockIndex                  uint
	BlockBottom                 uint
	BlockRight                  uint
	BlockTop                    uint
	BlockLeft                   uint
	CanvasIndex                 uint
	GalleryIdentificationNumber uint
}

// TODO rest of commands!
