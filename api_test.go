package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/garyburd/redigo/redis"
	"github.com/gorilla/mux"

	"github.com/TykTechnologies/tyk/apidef"
)

const apiTestDef = `{
	"id": "507f1f77bcf86cd799439011",
	"name": "Tyk Test API ONE",
	"api_id": "1",
	"org_id": "default",
	"definition": {
		"location": "header",
		"key": "version"
	},
	"auth": {
		"auth_header_name": "authorization"
	},
	"version_data": {
		"not_versioned": false,
		"versions": {
			"Default": {
				"name": "Default",
				"expires": "3006-01-02 15:04",
				"use_extended_paths": true,
				"paths": {
					"ignored": [],
					"white_list": [],
					"black_list": []
				}
			}
		}
	},
	"proxy": {
		"listen_path": "/v1",
		"target_url": "` + testHttpAny + `",
		"strip_listen_path": false
	}
}`

func makeSampleAPI(t *testing.T, def string) *APISpec {
	spec := createSpecTest(t, def)
	loadApps([]*APISpec{spec}, discardMuxer)
	return spec
}

type apiSuccess struct {
	Key    string `json:"key"`
	Status string `json:"status"`
	Action string `json:"action"`
}

type testAPIDefinition struct {
	apidef.APIDefinition
	ID string `json:"id"`
}

func TestHealthCheckEndpoint(t *testing.T) {
	uri := "/tyk/health/?api_id=1"
	method := "GET"

	recorder := httptest.NewRecorder()
	param := make(url.Values)

	makeSampleAPI(t, apiTestDef)

	req, err := http.NewRequest(method, uri+param.Encode(), nil)

	if err != nil {
		t.Fatal(err)
	}

	healthCheckhandler(recorder, req)

	var ApiHealthValues HealthCheckValues
	err = json.Unmarshal(recorder.Body.Bytes(), &ApiHealthValues)

	if err != nil {
		t.Error("Could not unmarshal API Health check:\n", err, recorder.Body.String())
	}

	if recorder.Code != 200 {
		t.Error("Recorder should return 200 for health check")
	}
}

func createSampleSession() SessionState {
	return SessionState{
		Rate:             5.0,
		Allowance:        5.0,
		LastCheck:        time.Now().Unix(),
		Per:              8.0,
		Expires:          0,
		QuotaRenewalRate: 300, // 5 minutes
		QuotaRenews:      time.Now().Unix(),
		QuotaRemaining:   10,
		QuotaMax:         10,
		AccessRights: map[string]AccessDefinition{
			"1": {
				APIName:  "Test",
				APIID:    "1",
				Versions: []string{"Default"},
			},
		},
	}
}

func TestApiHandler(t *testing.T) {
	uris := []string{"/tyk/apis/", "/tyk/apis"}

	for _, uri := range uris {
		method := "GET"
		sampleKey := createSampleSession()
		body, _ := json.Marshal(&sampleKey)

		recorder := httptest.NewRecorder()
		param := make(url.Values)

		makeSampleAPI(t, apiTestDef)

		req, err := http.NewRequest(method, uri+param.Encode(), bytes.NewReader(body))

		if err != nil {
			t.Fatal(err)
		}

		apiHandler(recorder, req)

		// We can't deserialize BSON ObjectID's if they are not in th test base!
		var ApiList []testAPIDefinition
		err = json.Unmarshal(recorder.Body.Bytes(), &ApiList)

		if err != nil {
			t.Error("Could not unmarshal API List:\n", err, recorder.Body.String(), uri)
		} else {
			if len(ApiList) != 1 {
				t.Error("API's not returned, len was: \n", len(ApiList), recorder.Body.String(), uri)
			} else {
				if ApiList[0].APIID != "1" {
					t.Error("Response is incorrect - no API ID value in struct :\n", recorder.Body.String(), uri)
				}
			}
		}
	}
}

func TestApiHandlerGetSingle(t *testing.T) {
	uri := "/tyk/apis/1"
	method := "GET"
	sampleKey := createSampleSession()
	body, _ := json.Marshal(&sampleKey)

	recorder := httptest.NewRecorder()
	param := make(url.Values)

	makeSampleAPI(t, apiTestDef)

	req, err := http.NewRequest(method, uri+param.Encode(), bytes.NewReader(body))

	if err != nil {
		t.Fatal(err)
	}

	apiHandler(recorder, req)

	// We can't deserialize BSON ObjectID's if they are not in th test base!
	var ApiDefinition testAPIDefinition
	err = json.Unmarshal(recorder.Body.Bytes(), &ApiDefinition)

	if err != nil {
		t.Error("Could not unmarshal API Definition:\n", err, recorder.Body.String())
	} else {
		if ApiDefinition.APIID != "1" {
			t.Error("Response is incorrect - no API ID value in struct :\n", recorder.Body.String())
		}
	}
}

func TestApiHandlerPost(t *testing.T) {
	uri := "/tyk/apis/1"
	method := "POST"

	recorder := httptest.NewRecorder()
	param := make(url.Values)

	req, err := http.NewRequest(method, uri+param.Encode(), strings.NewReader(apiTestDef))

	if err != nil {
		t.Fatal(err)
	}

	apiHandler(recorder, req)

	var success apiSuccess
	err = json.Unmarshal(recorder.Body.Bytes(), &success)

	if err != nil {
		t.Error("Could not unmarshal POST result:\n", err, recorder.Body.String())
	} else {
		if success.Status != "ok" {
			t.Error("Response is incorrect - not success :\n", recorder.Body.String())
		}
	}
}

func TestApiHandlerPostDbConfig(t *testing.T) {
	uri := "/tyk/apis/1"
	method := "POST"

	config.UseDBAppConfigs = true
	defer func() { config.UseDBAppConfigs = false }()

	recorder := httptest.NewRecorder()
	param := make(url.Values)

	req, err := http.NewRequest(method, uri+param.Encode(), strings.NewReader(apiTestDef))

	if err != nil {
		t.Fatal(err)
	}

	apiHandler(recorder, req)

	var success apiSuccess
	err = json.Unmarshal(recorder.Body.Bytes(), &success)

	if err != nil {
		t.Error("Could not unmarshal POST result:\n", err, recorder.Body.String())
	} else {
		if success.Status == "ok" {
			t.Error("Response is incorrect - expected error due to use_db_app_config :\n", recorder.Body.String())
		}
	}
}

func TestKeyHandlerNewKey(t *testing.T) {
	uri := "/tyk/keys/1234"
	method := "POST"
	sampleKey := createSampleSession()
	body, _ := json.Marshal(&sampleKey)

	recorder := httptest.NewRecorder()
	param := make(url.Values)

	makeSampleAPI(t, apiTestDef)
	param.Set("api_id", "1")
	req, err := http.NewRequest(method, uri+param.Encode(), bytes.NewReader(body))

	if err != nil {
		t.Fatal(err)
	}

	keyHandler(recorder, req)

	newSuccess := apiSuccess{}
	err = json.Unmarshal(recorder.Body.Bytes(), &newSuccess)

	if err != nil {
		t.Error("Could not unmarshal success message:\n", err)
	} else {
		if newSuccess.Status != "ok" {
			t.Error("key not created, status error:\n", recorder.Body.String())
		}
		if newSuccess.Action != "added" {
			t.Error("Response is incorrect - action is not 'added' :\n", recorder.Body.String())
		}
	}
}

func TestKeyHandlerUpdateKey(t *testing.T) {
	uri := "/tyk/keys/1234"
	method := "PUT"
	sampleKey := createSampleSession()
	body, _ := json.Marshal(&sampleKey)

	recorder := httptest.NewRecorder()
	param := make(url.Values)
	makeSampleAPI(t, apiTestDef)
	param.Set("api_id", "1")
	req, err := http.NewRequest(method, uri+param.Encode(), bytes.NewReader(body))

	if err != nil {
		t.Fatal(err)
	}

	keyHandler(recorder, req)

	newSuccess := apiSuccess{}
	err = json.Unmarshal(recorder.Body.Bytes(), &newSuccess)

	if err != nil {
		t.Error("Could not unmarshal success message:\n", err)
	} else {
		if newSuccess.Status != "ok" {
			t.Error("key not created, status error:\n", recorder.Body.String())
		}
		if newSuccess.Action != "modified" {
			t.Error("Response is incorrect - action is not 'modified' :\n", recorder.Body.String())
		}
	}
}

func TestKeyHandlerGetKey(t *testing.T) {
	makeSampleAPI(t, apiTestDef)
	createKey()

	uri := "/tyk/keys/1234"
	method := "GET"

	recorder := httptest.NewRecorder()
	param := make(url.Values)

	param.Set("api_id", "1")
	req, err := http.NewRequest(method, uri+"?"+param.Encode(), nil)

	if err != nil {
		t.Fatal(err)
	}

	keyHandler(recorder, req)

	newSuccess := make(map[string]interface{})
	err = json.Unmarshal(recorder.Body.Bytes(), &newSuccess)

	if err != nil {
		t.Error("Could not unmarshal success message:\n", err)
	} else {
		if recorder.Code != 200 {
			t.Error("key not requested, status error:\n", recorder.Body.String())
		}
	}
}

func TestKeyHandlerGetKeyNoAPIID(t *testing.T) {
	makeSampleAPI(t, apiTestDef)
	createKey()

	uri := "/tyk/keys/1234"
	method := "GET"

	recorder := httptest.NewRecorder()
	param := make(url.Values)

	req, err := http.NewRequest(method, uri+"?"+param.Encode(), nil)

	if err != nil {
		t.Fatal(err)
	}

	keyHandler(recorder, req)

	newSuccess := make(map[string]interface{})
	err = json.Unmarshal(recorder.Body.Bytes(), &newSuccess)

	if err != nil {
		t.Error("Could not unmarshal success message:\n", err)
	} else {
		if recorder.Code != 200 {
			t.Error("key not requested, status error:\n", recorder.Body.String())
		}
	}
}

func createKey() {
	uri := "/tyk/keys/1234"
	method := "POST"
	sampleKey := createSampleSession()
	body, _ := json.Marshal(&sampleKey)

	recorder := httptest.NewRecorder()
	param := make(url.Values)
	req, _ := http.NewRequest(method, uri+param.Encode(), bytes.NewReader(body))

	keyHandler(recorder, req)
}

func TestKeyHandlerDeleteKey(t *testing.T) {
	createKey()

	uri := "/tyk/keys/1234?"
	method := "DELETE"

	recorder := httptest.NewRecorder()
	param := make(url.Values)
	makeSampleAPI(t, apiTestDef)
	param.Set("api_id", "1")
	req, err := http.NewRequest(method, uri+param.Encode(), nil)

	if err != nil {
		t.Fatal(err)
	}

	keyHandler(recorder, req)

	newSuccess := apiSuccess{}
	err = json.Unmarshal(recorder.Body.Bytes(), &newSuccess)

	if err != nil {
		t.Error("Could not unmarshal success message:\n", err)
	} else {
		if newSuccess.Status != "ok" {
			t.Error("key not deleted, status error:\n", recorder.Body.String())
		}
		if newSuccess.Action != "deleted" {
			t.Error("Response is incorrect - action is not 'deleted' :\n", recorder.Body.String())
		}
	}
}

func TestCreateKeyHandlerCreateNewKey(t *testing.T) {
	createKey()

	uri := "/tyk/keys/create"
	method := "POST"

	sampleKey := createSampleSession()
	body, _ := json.Marshal(&sampleKey)

	recorder := httptest.NewRecorder()
	param := make(url.Values)
	makeSampleAPI(t, apiTestDef)
	param.Set("api_id", "1")
	req, err := http.NewRequest(method, uri+param.Encode(), bytes.NewReader(body))

	if err != nil {
		t.Fatal(err)
	}

	createKeyHandler(recorder, req)

	newSuccess := apiSuccess{}
	err = json.Unmarshal(recorder.Body.Bytes(), &newSuccess)

	if err != nil {
		t.Error("Could not unmarshal success message:\n", err)
	} else {
		if newSuccess.Status != "ok" {
			t.Error("key not created, status error:\n", recorder.Body.String())
		}
		if newSuccess.Action != "create" {
			t.Error("Response is incorrect - action is not 'create' :\n", recorder.Body.String())
		}
	}
}

func TestCreateKeyHandlerCreateNewKeyNoAPIID(t *testing.T) {
	createKey()

	uri := "/tyk/keys/create"
	method := "POST"

	sampleKey := createSampleSession()
	body, _ := json.Marshal(&sampleKey)

	recorder := httptest.NewRecorder()
	param := make(url.Values)
	makeSampleAPI(t, apiTestDef)
	req, err := http.NewRequest(method, uri+param.Encode(), bytes.NewReader(body))

	if err != nil {
		t.Fatal(err)
	}

	createKeyHandler(recorder, req)

	newSuccess := apiSuccess{}
	err = json.Unmarshal(recorder.Body.Bytes(), &newSuccess)

	if err != nil {
		t.Error("Could not unmarshal success message:\n", err)
	} else {
		if newSuccess.Status != "ok" {
			t.Error("key not created, status error:\n", recorder.Body.String())
		}
		if newSuccess.Action != "create" {
			t.Error("Response is incorrect - action is not 'create' :\n", recorder.Body.String())
		}
	}
}

func TestAPIAuthFail(t *testing.T) {

	uri := "/tyk/health/?api_id=1"
	method := "GET"

	recorder := httptest.NewRecorder()
	param := make(url.Values)
	req, err := http.NewRequest(method, uri+param.Encode(), nil)
	req.Header.Add("x-tyk-authorization", "12345")

	if err != nil {
		t.Fatal(err)
	}

	makeSampleAPI(t, apiTestDef)
	CheckIsAPIOwner(healthCheckhandler)(recorder, req)

	if recorder.Code == 200 {
		t.Error("Access to API should have been blocked, but response code was: ", recorder.Code)
	}
}

func TestAPIAuthOk(t *testing.T) {

	uri := "/tyk/health/?api_id=1"
	method := "GET"

	recorder := httptest.NewRecorder()
	param := make(url.Values)
	req, err := http.NewRequest(method, uri+param.Encode(), nil)
	req.Header.Add("x-tyk-authorization", "352d20ee67be67f6340b4c0605b044b7")

	if err != nil {
		t.Fatal(err)
	}

	makeSampleAPI(t, apiTestDef)
	CheckIsAPIOwner(healthCheckhandler)(recorder, req)

	if recorder.Code != 200 {
		t.Error("Access to API should have been blocked, but response code was: ", recorder.Code)
	}
}

func TestGetOAuthClients(t *testing.T) {
	var testAPIID = "1"
	var responseCode int

	_, responseCode = getOauthClients(testAPIID)
	if responseCode != 400 {
		t.Fatal("Retrieving OAuth clients from nonexistent APIs must return error.")
	}

	ApiSpecRegister = make(map[string]*APISpec)
	ApiSpecRegister[testAPIID] = &APISpec{}

	_, responseCode = getOauthClients(testAPIID)
	if responseCode != 400 {
		t.Fatal("Retrieving OAuth clients from APIs with no OAuthManager must return an error.")
	}

	ApiSpecRegister = nil
}

func TestResetHandler(t *testing.T) {
	uri := "/tyk/reload/"

	ApiSpecRegister = make(map[string]*APISpec)

	makeSampleAPI(t, apiTestDef)

	recorder := httptest.NewRecorder()
	params := make(url.Values)

	req, err := http.NewRequest("GET", uri+params.Encode(), nil)

	if err != nil {
		t.Fatal(err)
	}

	resetHandler(recorder, req)

	if recorder.Code != 200 {
		t.Fatal("Hot reload failed, response code was: ", recorder.Code)
	}

	if len(ApiSpecRegister) == 0 {
		t.Fatal("Hot reload was triggered but no APIs were found.")
	}
}

func TestGroupResetHandler(t *testing.T) {
	signalChan := make(chan bool)
	cacheStore := RedisClusterStorageManager{}
	cacheStore.Connect()

	go func() {
		cacheStore.StartPubSubHandler(RedisPubSubChannel, func(message redis.Message) {
			notif := Notification{}
			if err := json.Unmarshal(message.Data, &notif); err != nil {
				t.Fatal("Unmarshalling message body failed, malformed: ", err)
			}
			if notif.Command == NoticeGroupReload {
				signalChan <- true
			} else {
				signalChan <- false
			}
		})
	}()

	uri := "/tyk/reload/group"

	ApiSpecRegister = make(map[string]*APISpec)

	makeSampleAPI(t, apiTestDef)

	recorder := httptest.NewRecorder()
	params := make(url.Values)

	req, err := http.NewRequest("GET", uri+params.Encode(), nil)

	if err != nil {
		t.Fatal(err)
	}

	groupResetHandler(recorder, req)

	if recorder.Code != 200 {
		t.Fatal("Hot reload (group) failed, response code was: ", recorder.Code)
	}

	if len(ApiSpecRegister) == 0 {
		t.Fatal("Hot reload (group) was triggered but no APIs were found.")
	}

	// We wait for the right notification (NoticeGroupReload), other type of notifications may be received during tests, as this is the cluster channel:
	for {
		recvSignal := <-signalChan
		if recvSignal {
			break
		}
	}

}

const apiBenchDef = `{
	"name": "Bench API",
	"api_id": "REPLACE",
	"org_id": "default",
	"definition": {
		"location": "header",
		"key": "version"
	},
	"auth": {
		"auth_header_name": "authorization"
	},
	"version_data": {
		"not_versioned": true,
		"versions": {
			"Default": {
				"name": "Default",
				"use_extended_paths": true
			}
		}
	},
	"proxy": {
		"listen_path": "/listen-REPLACE",
		"target_url": "` + testHttpAny + `",
		"strip_listen_path": false
	}
}`

func BenchmarkApiInsertReload(b *testing.B) {
	redisStore := &RedisClusterStorageManager{KeyPrefix: "apikey."}
	healthStore := &RedisClusterStorageManager{KeyPrefix: "apihealth."}
	orgStore := &RedisClusterStorageManager{KeyPrefix: "orgKey."}

	specs := make([]*APISpec, 1000)
	for i := range specs {
		id := strconv.Itoa(i + 1)
		def := strings.Replace(apiBenchDef, "REPLACE", id, -1)
		spec := createDefinitionFromString(def)
		spec.Init(redisStore, redisStore, healthStore, orgStore)
		specs[i] = spec
	}

	for i := 0; i < b.N; i++ {
		newMuxes := mux.NewRouter()
		loadAPIEndpoints(newMuxes)
		loadApps(specs, newMuxes)
	}
}
