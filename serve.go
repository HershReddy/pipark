package pipark2014

import (
	"appengine"
	"appengine/datastore"
	//"appengine/user"
	"html/template"
	"mux"
	"net/http"
	"rpc"
	"rpc/json"
	"time"
	//"bufio"
	//"fmt"
	//"log"
	//"os"
)

const (
	imageHeight                  = 480
	imageWidth                   = 640
	maximumPictureUpdatesPerHour = 200
)

type FormData struct {
	Location   string
	ImageURL   string
	GeoLoc     string
	Height     int
	Width      int
	NumCameras int
	Year       int
	Month      time.Month
	Day        int
}

type RasPiCamState struct {
	Location            string
	LastPing            time.Time
	LastImageUpdate     time.Time
	MonitorStart        time.Time
	LatestImageURL      string
	RequestImageUpdate  bool
	NumUpdatesMonitored int
}

func init() {
	r := mux.NewRouter()
	r.HandleFunc("/view/{location}", viewHandler)
	r.HandleFunc("/request/{location}", requestHandler)
	r.HandleFunc("/test", testHandler)
	r.HandleFunc("/clientcheck/{location}", clientCheckHandler)
	r.HandleFunc("/clientupdate/{location}", clientUpdateHandler)
	r.HandleFunc("/", rootHandler)
	http.Handle("/", r)
}

func getLocalCameras(c *appengine.Context, r *http.Request) ([]*datastore.Key, []RasPiCamState, string, error) {
	// construct a query to get all the cameras for the location.
	vars := mux.Vars(r)
	location := vars["location"]
	q := datastore.NewQuery("RasPiCamState").Filter("Location =", location)
	// run the query
	var cams []RasPiCamState
	keys, err := q.GetAll(*c, &cams)

	return keys, cams, location, err
}

func viewHandler(w http.ResponseWriter, r *http.Request) {
	c := appengine.NewContext(r)

	_, cams, location, err := getLocalCameras(&c, r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if len(cams) < 1 {
		errstring := "Server found no cameras active at " + location
		http.Error(w, errstring, http.StatusInternalServerError)
		return
	}

	// set up a data structure containing values used to render out the location template
	formTemplate, _ := template.ParseFiles("html/view.html")
	formdata := FormData{
		Location:   location,
		ImageURL:   cams[0].LatestImageURL,
		GeoLoc:     location,
		Height:     imageHeight,
		Width:      imageWidth,
		NumCameras: len(cams),
	}
	formdata.Year, formdata.Month, formdata.Day = cams[0].LastImageUpdate.Date()

	if err := formTemplate.Execute(w, formdata); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func newCamState(c *appengine.Context, w http.ResponseWriter, r *http.Request, location string) {
	vars := mux.Vars(r)
	latestimageURL := vars["LatestImageURL"]

	rcs := RasPiCamState{
		Location:            location,
		LastPing:            time.Now(),
		LastImageUpdate:     time.Now(),
		MonitorStart:        time.Now(),
		LatestImageURL:      latestimageURL,
		RequestImageUpdate:  false,
		NumUpdatesMonitored: 1,
	}

	_, err := datastore.Put(*c, datastore.NewIncompleteKey(*c, "RasPiCamState", nil), &rcs)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func updateCamState(c *appengine.Context, w http.ResponseWriter, r *http.Request, key *datastore.Key, rcs *RasPiCamState) {
	vars := mux.Vars(r)
	latestimageURL := vars["LatestImageURL"]

	var (
		timeMon time.Time
		numUps  int
	)

	if time.Since(rcs.MonitorStart).Hours() > 1.0 {
		timeMon = time.Now()
		numUps = 1
	} else {
		timeMon = rcs.MonitorStart
		numUps = rcs.NumUpdatesMonitored + 1
	}

	rcsnew := RasPiCamState{
		Location:            rcs.Location,
		LastPing:            time.Now(),
		LastImageUpdate:     time.Now(),
		MonitorStart:        timeMon,
		LatestImageURL:      latestimageURL,
		RequestImageUpdate:  false,
		NumUpdatesMonitored: numUps,
	}

	_, err := datastore.Put(*c, key, rcsnew)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func clientUpdateHandler(w http.ResponseWriter, r *http.Request) {
	c := appengine.NewContext(r)
	if r.Method != "POST" {
		http.Error(w, "Upload endpoint only supports POST method.", http.StatusMethodNotAllowed)
		return
	}
	keys, cams, location, err := getLocalCameras(&c, r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	numcams := len(cams)
	switch {
	case numcams <= 0:
		newCamState(&c, w, r, location)
	case numcams >= 1:
		updateCamState(&c, w, r, keys[0], &cams[0])
	}

}

func rootHandler(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "html/root.html")
}

func testHandler(w http.ResponseWriter, r *http.Request) {
	c := appengine.NewContext(r)

	rcs := RasPiCamState{
		Location:            "300ThirdStreet",
		LastPing:            time.Now(),
		LastImageUpdate:     time.Now(),
		MonitorStart:        time.Now(),
		LatestImageURL:      "http://storage.googleapis.com/pipark2014/parkingspots/imgs/300third.jpg",
		RequestImageUpdate:  false,
		NumUpdatesMonitored: 1,
	}

	_, err := datastore.Put(c, datastore.NewIncompleteKey(c, "RasPiCamState", nil), &rcs)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/", http.StatusOK)
}

func requestHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	location := vars["location"]
	c := appengine.NewContext(r)
	keys, cams, location, err := getLocalCameras(&c, r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if len(cams) < 1 {
		http.Error(w, "Server found no cameras active at this location.", http.StatusInternalServerError)
		return
	}

	rcs := cams[0]
	key := keys[0]

	if rcs.NumUpdatesMonitored > maximumPictureUpdatesPerHour {
		http.Error(w, "Update quota for this camera location has been exceeded.  Please check back soon.", http.StatusInternalServerError)
		return
	}

	// Create a new cam state that will replace the old cam state.
	// The new cam state will indicate that RequestImageUpdate is true.
	// The next time a Raspberry Pi at the location polls the server
	// it will check the state of RequestImageUpdate and will know that
	// it needs to upload a new picture of the location.
	rcsnew := RasPiCamState{
		Location:            rcs.Location,
		LastPing:            rcs.LastPing,
		LastImageUpdate:     rcs.LastImageUpdate,
		MonitorStart:        rcs.MonitorStart,
		LatestImageURL:      rcs.LatestImageURL,
		RequestImageUpdate:  true,
		NumUpdatesMonitored: rcs.NumUpdatesMonitored,
	}

	_, err = datastore.Put(c, key, &rcsnew)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	redirectURL := "/view/" + location
	http.Redirect(w, r, redirectURL, http.StatusOK)
	return
}