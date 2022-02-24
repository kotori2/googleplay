package googleplay

import (
   "github.com/89z/format/protobuf"
   "net/http"
   "net/url"
   "strconv"
   "strings"
)

type Header struct {
   http.Header
}

// This should work up to 617.
func (h Header) Category(cat string, min int) ([]Document, error) {
   var (
      docs []Document
      done int
      next string
   )
   for done < min {
      var (
         doct []Document
         err error
      )
      if done == 0 {
         doct, next, err = h.documents(cat, "")
      } else {
         doct, next, err = h.documents("", next)
      }
      if err != nil {
         return nil, err
      }
      docs = append(docs, doct...)
      // On the last page, you will have some results, and empty URL.
      if next == "" {
         break
      }
      done += len(doct)
   }
   return docs, nil
}

func (h Header) Delivery(app string, ver int64) (*Delivery, error) {
   req, err := http.NewRequest(
      "GET", "https://play-fe.googleapis.com/fdfe/delivery", nil,
   )
   if err != nil {
      return nil, err
   }
   req.Header = h.Header
   req.URL.RawQuery = url.Values{
      "doc": {app},
      "vc": {strconv.FormatInt(ver, 10)},
   }.Encode()
   LogLevel.Dump(req)
   res, err := new(http.Transport).RoundTrip(req)
   if err != nil {
      return nil, err
   }
   defer res.Body.Close()
   responseWrapper, err := protobuf.Decode(res.Body)
   if err != nil {
      return nil, err
   }
   status := responseWrapper.Get(1, "payload").
      Get(21, "deliveryResponse").
      GetVarint(1, "status")
   switch status {
   case 2:
      return nil, errorString("Geo-blocking")
   case 3:
      return nil, errorString("Purchase required")
   case 5:
      return nil, errorString("Invalid version")
   }
   appData := responseWrapper.Get(1, "payload").
      Get(21, "deliveryResponse").
      Get(2, "appDeliveryData")
   var del Delivery
   del.DownloadURL = appData.GetString(3, "downloadUrl")
   for _, data := range appData.GetMessages(15, "splitDeliveryData") {
      var split SplitDeliveryData
      split.ID = data.GetString(1, "id")
      split.DownloadURL = data.GetString(5, "downloadUrl")
      del.SplitDeliveryData = append(del.SplitDeliveryData, split)
   }
   return &del, nil
}

func (h Header) Details(app string) (*Details, error) {
   req, err := http.NewRequest(
      "GET", "https://android.clients.google.com/fdfe/details", nil,
   )
   if err != nil {
      return nil, err
   }
   req.Header = h.Header
   req.URL.RawQuery = "doc=" + url.QueryEscape(app)
   LogLevel.Dump(req)
   res, err := new(http.Transport).RoundTrip(req)
   if err != nil {
      return nil, err
   }
   if res.StatusCode != http.StatusOK {
      return nil, errorString(res.Status)
   }
   responseWrapper, err := protobuf.Decode(res.Body)
   if err != nil {
      return nil, err
   }
   docV2 := responseWrapper.Get(1, "payload").
      Get(2, "detailsResponse").
      Get(4, "docV2")
   var det Details
   det.CurrencyCode = docV2.Get(8, "offer").GetString(2, "currencyCode")
   det.Micros = docV2.Get(8, "offer").GetVarint(1, "micros")
   det.NumDownloads = docV2.Get(13, "details").
      Get(1, "appDetails").
      GetVarint(70, "numDownloads")
   // The shorter path 13,1,9 returns wrong size for some packages:
   // com.riotgames.league.wildriftvn
   det.Size = docV2.Get(13, "details").
      Get(1, "appDetails").
      Get(34, "installDetails").
      GetVarint(2, "size")
   det.Title = docV2.GetString(5, "title")
   det.UploadDate = docV2.Get(13, "details").
      Get(1, "appDetails").
      GetString(16, "uploadDate")
   det.VersionCode = docV2.Get(13, "details").
      Get(1, "appDetails").
      GetVarint(3, "versionCode")
   det.VersionString = docV2.Get(13, "details").
      Get(1, "appDetails").
      GetString(4, "versionString")
   return &det, nil
}

// Purchase app. Only needs to be done once per Google account.
func (h Header) Purchase(app string) error {
   query := "doc=" + url.QueryEscape(app)
   req, err := http.NewRequest(
      "POST", "https://android.clients.google.com/fdfe/purchase",
      strings.NewReader(query),
   )
   if err != nil {
      return err
   }
   h.Set("Content-Type", "application/x-www-form-urlencoded")
   req.Header = h.Header
   LogLevel.Dump(req)
   res, err := new(http.Transport).RoundTrip(req)
   if err != nil {
      return err
   }
   return res.Body.Close()
}

// You can also use "/fdfe/browse", but it uses "preFetch".
// You can also use "/fdfe/homeV2", but it uses "preFetch".
// You can also use "/fdfe/listTopChartItems" as an alias for "/fdfe/list".
func (h Header) documents(cat, next string) ([]Document, string, error) {
   var buf strings.Builder
   buf.WriteString("https://android.clients.google.com/fdfe/")
   if cat != "" {
      buf.WriteString("list?")
      val := url.Values{
         "c": {"3"},
         "cat": {cat},
         "ctr": {"apps_topselling_free"},
      }.Encode()
      buf.WriteString(val)
   } else {
      buf.WriteString(next)
   }
   req, err := http.NewRequest("GET", buf.String(), nil)
   if err != nil {
      return nil, "", err
   }
   req.Header = h.Header
   LogLevel.Dump(req)
   res, err := new(http.Transport).RoundTrip(req)
   if err != nil {
      return nil, "", err
   }
   defer res.Body.Close()
   responseWrapper, err := protobuf.Decode(res.Body)
   if err != nil {
      return nil, "", err
   }
   docV2 := responseWrapper.Get(1, "payload").
      Get(1, "listResponse").
      Get(2, "doc").
      Get(11, "child")
   var docs []Document
   for _, child := range docV2.GetMessages(11, "child") {
      var doc Document
      doc.ID = child.GetString(1, "docID")
      doc.Title = child.GetString(5, "title")
      doc.Creator = child.GetString(6, "creator")
      docs = append(docs, doc)
   }
   next = docV2.Get(12, "containerMetadata").GetString(2, "nextPageUrl")
   return docs, next, nil
}
