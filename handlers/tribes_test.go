package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stakwork/sphinx-tribes/config"

	"github.com/go-chi/chi"
	"github.com/lib/pq"
	"github.com/stakwork/sphinx-tribes/auth"
	"github.com/stakwork/sphinx-tribes/db"
	mocks "github.com/stakwork/sphinx-tribes/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestGetTribesByOwner(t *testing.T) {
	mockDb := mocks.NewDatabase(t)
	tHandler := NewTribeHandler(mockDb)

	t.Run("Should test that all tribes that an owner did not delete are returned if all=true is added to the request query", func(t *testing.T) {
		// Mock data
		mockPubkey := "mock_pubkey"
		mockTribes := []db.Tribe{
			{UUID: "uuid", OwnerPubKey: mockPubkey, Deleted: false},
			{UUID: "uuid", OwnerPubKey: mockPubkey, Deleted: false},
		}
		mockDb.On("GetAllTribesByOwner", mock.Anything).Return(mockTribes).Once()

		// Create request with "all=true" query parameter
		req, err := http.NewRequest("GET", "/tribes_by_owner/"+mockPubkey+"?all=true", nil)
		if err != nil {
			t.Fatal(err)
		}

		// Serve request
		rr := httptest.NewRecorder()
		handler := http.HandlerFunc(tHandler.GetTribesByOwner)
		handler.ServeHTTP(rr, req)

		// Verify response
		assert.Equal(t, http.StatusOK, rr.Code)
		var responseData []db.Tribe
		err = json.Unmarshal(rr.Body.Bytes(), &responseData)
		if err != nil {
			t.Fatalf("Error decoding JSON response: %s", err)
		}
		assert.ElementsMatch(t, mockTribes, responseData)
	})

	t.Run("Should test that all tribes that are not unlisted by an owner are returned", func(t *testing.T) {
		// Mock data
		mockPubkey := "mock_pubkey"
		mockTribes := []db.Tribe{
			{UUID: "uuid", OwnerPubKey: mockPubkey, Unlisted: false},
			{UUID: "uuid", OwnerPubKey: mockPubkey, Unlisted: false},
		}
		mockDb.On("GetTribesByOwner", mock.Anything).Return(mockTribes)

		// Create request without "all=true" query parameter
		req, err := http.NewRequest("GET", "/tribes/"+mockPubkey, nil)
		if err != nil {
			t.Fatal(err)
		}

		// Serve request
		rr := httptest.NewRecorder()
		handler := http.HandlerFunc(tHandler.GetTribesByOwner)
		handler.ServeHTTP(rr, req)

		// Verify response
		assert.Equal(t, http.StatusOK, rr.Code)
		var responseData []db.Tribe
		err = json.Unmarshal(rr.Body.Bytes(), &responseData)
		if err != nil {
			t.Fatalf("Error decoding JSON response: %s", err)
		}
		assert.ElementsMatch(t, mockTribes, responseData)
	})
}

func TestGetTribe(t *testing.T) {
	teardownSuite := SetupSuite(t)
	defer teardownSuite(t)
	tHandler := NewTribeHandler(db.TestDB)

	tribe := db.Tribe{
		UUID:        uuid.New().String(),
		OwnerPubKey: uuid.New().String(),
		Name:        "tribe",
		Description: "description",
		Tags:        []string{"tag1", "tag2"},
		Badges:      pq.StringArray{},
	}
	db.TestDB.CreateOrEditTribe(tribe)

	t.Run("Should test that a tribe can be returned when the right UUID is passed to the request parameter", func(t *testing.T) {
		// Mock data
		mockUUID := tribe.UUID

		// Serve request
		rr := httptest.NewRecorder()
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("uuid", mockUUID)
		req, err := http.NewRequestWithContext(context.WithValue(context.Background(), chi.RouteCtxKey, rctx), http.MethodGet, "/"+mockUUID, nil)
		if err != nil {
			t.Fatal(err)
		}

		fetchedTribe := db.TestDB.GetTribe(mockUUID)

		handler := http.HandlerFunc(tHandler.GetTribe)
		handler.ServeHTTP(rr, req)

		// Verify response
		assert.Equal(t, http.StatusOK, rr.Code)
		var responseData map[string]interface{}
		err = json.Unmarshal(rr.Body.Bytes(), &responseData)
		if err != nil {
			t.Fatalf("Error decoding JSON response: %s", err)
		}
		assert.Equal(t, tribe.UUID, responseData["uuid"])
		assert.Equal(t, tribe, fetchedTribe)
	})

	t.Run("Should test that no tribe is returned when a nonexistent UUID is passed", func(t *testing.T) {

		nonexistentUUID := "nonexistent_uuid"

		// Serve request
		rr := httptest.NewRecorder()
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("uuid", nonexistentUUID)
		req, err := http.NewRequestWithContext(context.WithValue(context.Background(), chi.RouteCtxKey, rctx), http.MethodGet, "/"+nonexistentUUID, nil)
		if err != nil {
			t.Fatal(err)
		}

		handler := http.HandlerFunc(tHandler.GetTribe)
		handler.ServeHTTP(rr, req)

		// Verify response
		assert.Equal(t, http.StatusOK, rr.Code)
		var responseData map[string]interface{}
		err = json.Unmarshal(rr.Body.Bytes(), &responseData)
		if err != nil {
			t.Fatalf("Error decoding JSON response: %s", err)
		}
		assert.Equal(t, "", responseData["uuid"])
	})
}

func TestGetTribesByAppUrl(t *testing.T) {
	teardownSuite := SetupSuite(t)
	defer teardownSuite(t)

	tHandler := NewTribeHandler(db.TestDB)

	t.Run("Should test that a tribe is returned when the right app URL is passed", func(t *testing.T) {
		tribe := db.Tribe{
			UUID:        "uuid",
			OwnerPubKey: "pubkey",
			Name:        "name",
			Description: "description",
			Tags:        []string{"tag3", "tag4"},
			AppURL:      "valid_app_url",
			Badges:      []string{},
		}
		db.TestDB.CreateOrEditTribe(tribe)

		// Mock data
		mockAppURL := tribe.AppURL
		mockTribes := []db.Tribe{tribe}

		// Serve request
		rr := httptest.NewRecorder()
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("app_url", mockAppURL)
		req, err := http.NewRequestWithContext(context.WithValue(context.Background(), chi.RouteCtxKey, rctx), http.MethodGet, "/app_url/"+mockAppURL, nil)
		if err != nil {
			t.Fatal(err)
		}

		handler := http.HandlerFunc(tHandler.GetTribesByAppUrl)
		handler.ServeHTTP(rr, req)

		// Verify response
		assert.Equal(t, http.StatusOK, rr.Code)
		var responseData []db.Tribe
		err = json.Unmarshal(rr.Body.Bytes(), &responseData)
		if err != nil {
			t.Fatalf("Error decoding JSON response: %s", err)
		}
		assert.ElementsMatch(t, mockTribes, responseData)
	})
}

func TestDeleteTribe(t *testing.T) {
	teardownSuite := SetupSuite(t)
	defer teardownSuite(t)

	personUUID := uuid.New().String()
	person := db.Person{
		Uuid:        personUUID,
		OwnerAlias:  "person_alias",
		UniqueName:  "person_unique_name",
		OwnerPubKey: "owner_pubkey",
		PriceToMeet: 0,
		Description: "this is test user 1",
	}
	db.TestDB.CreateOrEditPerson(person)

	tribeUUID := uuid.New().String()
	tribe := db.Tribe{
		UUID:        tribeUUID,
		OwnerPubKey: person.OwnerPubKey,
		Name:        "tribe_name",
		Description: "description",
		Tags:        []string{"tag3", "tag4"},
		AppURL:      "tribe_app_url",
	}
	db.TestDB.CreateOrEditTribe(tribe)

	tHandler := NewTribeHandler(db.TestDB)

	t.Run("Should test that the owner of a tribe can delete a tribe", func(t *testing.T) {
		mockUUID := tribe.AppURL
		mockOwnerPubKey := person.OwnerPubKey

		mockVerifyTribeUUID := func(uuid string, checkTimestamp bool) (string, error) {
			return mockOwnerPubKey, nil
		}
		tHandler.verifyTribeUUID = mockVerifyTribeUUID

		ctx := context.WithValue(context.Background(), auth.ContextKey, mockOwnerPubKey)
		rr := httptest.NewRecorder()
		handler := http.HandlerFunc(tHandler.DeleteTribe)

		req, err := http.NewRequestWithContext(ctx, "DELETE", "/tribe/"+mockUUID, nil)
		if err != nil {
			t.Fatal(err)
		}
		chiCtx := chi.NewRouteContext()
		chiCtx.URLParams.Add("uuid", tribeUUID)
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, chiCtx))

		handler.ServeHTTP(rr, req)

		// Verify response
		assert.Equal(t, http.StatusOK, rr.Code)
		var responseData bool
		err = json.Unmarshal(rr.Body.Bytes(), &responseData)
		assert.NoError(t, err)
		assert.True(t, responseData)

		// Assert that the tribe is deleted from the DB
		deletedTribe := db.TestDB.GetTribe(tribeUUID)
		assert.NoError(t, err)
		assert.Empty(t, deletedTribe)
		assert.Equal(t, db.Tribe{}, deletedTribe)
	})

	t.Run("Should test that a 401 error is returned when a tribe is attempted to be deleted by someone other than the owner", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), auth.ContextKey, "other_pubkey")
		mockUUID := tribe.AppURL
		mockOwnerPubKey := person.OwnerPubKey

		mockVerifyTribeUUID := func(uuid string, checkTimestamp bool) (string, error) {
			return mockOwnerPubKey, nil
		}
		tHandler.verifyTribeUUID = mockVerifyTribeUUID

		rr := httptest.NewRecorder()
		handler := http.HandlerFunc(tHandler.DeleteTribe)

		req, err := http.NewRequestWithContext(ctx, "DELETE", "/tribe/"+mockUUID, nil)
		if err != nil {
			t.Fatal(err)
		}
		chiCtx := chi.NewRouteContext()
		chiCtx.URLParams.Add("uuid", tribeUUID)
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, chiCtx))

		handler.ServeHTTP(rr, req)

		// Verify response
		assert.Equal(t, http.StatusUnauthorized, rr.Code)
	})
}

func TestGetFirstTribeByFeed(t *testing.T) {
	mockDb := mocks.NewDatabase(t)
	tHandler := NewTribeHandler(mockDb)

	t.Run("Should test that a tribe can be gotten by passing the feed URL", func(t *testing.T) {
		// Mock data
		mockFeedURL := "valid_feed_url"
		mockTribe := db.Tribe{
			UUID: "valid_uuid",
		}
		mockChannels := []db.Channel{
			{ID: 1, TribeUUID: mockTribe.UUID},
		}

		mockDb.On("GetFirstTribeByFeedURL", mockFeedURL).Return(mockTribe).Once()
		mockDb.On("GetChannelsByTribe", mockTribe.UUID).Return(mockChannels).Once()

		// Create request with valid feed URL
		req, err := http.NewRequest("GET", "/tribe_by_feed?url="+mockFeedURL, nil)
		if err != nil {
			t.Fatal(err)
		}

		// Serve request
		rr := httptest.NewRecorder()
		handler := http.HandlerFunc(tHandler.GetFirstTribeByFeed)
		handler.ServeHTTP(rr, req)

		// Verify response
		assert.Equal(t, http.StatusOK, rr.Code)
		var responseData map[string]interface{}
		err = json.Unmarshal(rr.Body.Bytes(), &responseData)
		if err != nil {
			t.Fatalf("Error decoding JSON response: %s", err)
		}
		assert.Equal(t, mockTribe.UUID, responseData["uuid"])
	})
}

func TestSetTribePreview(t *testing.T) {
	ctx := context.WithValue(context.Background(), auth.ContextKey, "owner_pubkey")
	mockDb := mocks.NewDatabase(t)
	tHandler := NewTribeHandler(mockDb)

	t.Run("Should test that the owner of a tribe can set tribe preview", func(t *testing.T) {
		// Mock data
		mockUUID := "valid_uuid"
		mockOwnerPubKey := "owner_pubkey"

		mockVerifyTribeUUID := func(uuid string, checkTimestamp bool) (string, error) {
			return mockOwnerPubKey, nil
		}
		mockDb.On("UpdateTribe", mock.Anything, map[string]interface{}{"preview": "preview"}).Return(true)

		tHandler.verifyTribeUUID = mockVerifyTribeUUID

		// Create and serve request
		rr := httptest.NewRecorder()
		handler := http.HandlerFunc(tHandler.SetTribePreview)

		req, err := http.NewRequestWithContext(ctx, "PUT", "/tribepreview/"+mockUUID+"?preview=preview", nil)
		if err != nil {
			t.Fatal(err)
		}
		chiCtx := chi.NewRouteContext()
		chiCtx.URLParams.Add("uuid", "mockUUID")
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, chiCtx))

		handler.ServeHTTP(rr, req)

		// Verify response
		assert.Equal(t, http.StatusOK, rr.Code)
		var responseData bool
		errors := json.Unmarshal(rr.Body.Bytes(), &responseData)
		assert.NoError(t, errors)
		assert.True(t, responseData)
	})

	t.Run("Should test that a 401 error is returned when setting a tribe preview action by someone other than the owner", func(t *testing.T) {
		// Mock data
		ctx := context.WithValue(context.Background(), auth.ContextKey, "pubkey")
		mockUUID := "valid_uuid"
		mockOwnerPubKey := "owner_pubkey"

		mockVerifyTribeUUID := func(uuid string, checkTimestamp bool) (string, error) {
			return mockOwnerPubKey, nil
		}

		tHandler.verifyTribeUUID = mockVerifyTribeUUID

		// Create and serve request
		rr := httptest.NewRecorder()
		handler := http.HandlerFunc(tHandler.SetTribePreview)

		req, err := http.NewRequestWithContext(ctx, "PUT", "/tribepreview/"+mockUUID+"?preview=preview", nil)
		if err != nil {
			t.Fatal(err)
		}
		chiCtx := chi.NewRouteContext()
		chiCtx.URLParams.Add("uuid", "mockUUID")
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, chiCtx))

		handler.ServeHTTP(rr, req)

		// Verify response
		assert.Equal(t, http.StatusUnauthorized, rr.Code)
	})
}

func TestCreateOrEditTribe(t *testing.T) {
	teardownSuite := SetupSuite(t)
	defer teardownSuite(t)

	tHandler := NewTribeHandler(db.TestDB)

	t.Run("Should test that a tribe can be created when the right data is passed", func(t *testing.T) {

		tribe := db.Tribe{
			UUID:        "uuid",
			OwnerPubKey: "pubkey",
			Name:        "name",
			Description: "description",
			Tags:        []string{"tag3", "tag4"},
			AppURL:      "valid_app_url",
			Badges:      []string{},
		}

		requestBody := map[string]interface{}{
			"UUID":        tribe.UUID,
			"OwnerPubkey": tribe.OwnerPubKey,
			"Name":        tribe.Name,
			"Description": tribe.Description,
			"Tags":        tribe.Tags,
			"AppURL":      tribe.AppURL,
			"Badges":      tribe.Badges,
		}
		mockVerifyTribeUUID := func(uuid string, checkTimestamp bool) (string, error) {
			return tribe.OwnerPubKey, nil
		}

		tHandler.verifyTribeUUID = mockVerifyTribeUUID

		requestBodyBytes, err := json.Marshal(requestBody)
		if err != nil {
			t.Fatal(err)
		}

		req, err := http.NewRequest("POST", "/", bytes.NewBuffer(requestBodyBytes))
		if err != nil {
			t.Fatal(err)
		}

		ctx := context.WithValue(req.Context(), auth.ContextKey, tribe.OwnerPubKey)
		req = req.WithContext(ctx)

		rr := httptest.NewRecorder()
		handler := http.HandlerFunc(tHandler.CreateOrEditTribe)
		handler.ServeHTTP(rr, req)

		// Verify response
		assert.Equal(t, http.StatusOK, rr.Code)
		var responseData map[string]interface{}
		err = json.Unmarshal(rr.Body.Bytes(), &responseData)
		if err != nil {
			t.Fatalf("Error decoding JSON response: %s", err)
		}

		// Assert that the response data is equal to the tribe POST data sent to the request
		assert.Equal(t, tribe.UUID, responseData["uuid"])
		assert.Equal(t, tribe.Name, responseData["name"])
		assert.Equal(t, tribe.Description, responseData["description"])
		assert.ElementsMatch(t, tribe.Tags, responseData["tags"])
		assert.Equal(t, tribe.OwnerPubKey, responseData["owner_pubkey"])
	})
}

func TestGetTribeByUniqueName(t *testing.T) {
	teardownSuite := SetupSuite(t)
	defer teardownSuite(t)

	tHandler := NewTribeHandler(db.TestDB)

	t.Run("Should test that a tribe can be fetched by its unique name", func(t *testing.T) {

		tribe := db.Tribe{
			UUID:        "uuid",
			OwnerPubKey: "pubkey",
			Name:        "name",
			UniqueName:  "test_tribe",
			Description: "description",
			Tags:        []string{"tag3", "tag4"},
			AppURL:      "valid_app_url",
			Badges:      []string{},
		}
		db.TestDB.CreateOrEditTribe(tribe)

		mockUniqueName := tribe.UniqueName

		rr := httptest.NewRecorder()
		rctx := chi.NewRouteContext()

		rctx.URLParams.Add("un", mockUniqueName)
		req, err := http.NewRequestWithContext(context.WithValue(context.Background(), chi.RouteCtxKey, rctx), http.MethodGet, "/tribe_by_un/"+mockUniqueName, nil)
		if err != nil {
			t.Fatal(err)
		}

		handler := http.HandlerFunc(tHandler.GetTribeByUniqueName)
		handler.ServeHTTP(rr, req)

		// Verify response
		assert.Equal(t, http.StatusOK, rr.Code)

		var responseData map[string]interface{}
		err = json.Unmarshal(rr.Body.Bytes(), &responseData)
		if err != nil {
			t.Fatalf("Error decoding JSON response: %s", err)
		}
		assert.Equal(t, mockUniqueName, responseData["unique_name"])
		assert.Equal(t, tribe.UUID, responseData["uuid"])
		assert.Equal(t, tribe.Name, responseData["name"])
		assert.Equal(t, tribe.Description, responseData["description"])
		assert.ElementsMatch(t, tribe.Tags, responseData["tags"])
	})
}

func TestGetAllTribes(t *testing.T) {
	teardownSuite := SetupSuite(t)
	defer teardownSuite(t)

	tHandler := NewTribeHandler(db.TestDB)
	t.Run("should return all tribes", func(t *testing.T) {
		rr := httptest.NewRecorder()
		handler := http.HandlerFunc(tHandler.GetAllTribes)

		db.TestDB.DeleteTribe()

		tribe := db.Tribe{
			UUID:        "uuid",
			OwnerPubKey: "pubkey",
			Name:        "name",
			UniqueName:  "uniqueName",
			Description: "description",
			Tags:        []string{"tag3", "tag4"},
			AppURL:      "AppURl",
			Badges:      []string{},
		}

		tribe2 := db.Tribe{
			UUID:        "uuid2",
			OwnerPubKey: "pubkey2",
			Name:        "name2",
			UniqueName:  "uniqueName2",
			Description: "description2",
			Tags:        []string{"tag3", "tag4"},
			AppURL:      "AppURl2",
			Badges:      []string{},
		}

		db.TestDB.CreateOrEditTribe(tribe)
		db.TestDB.CreateOrEditTribe(tribe2)

		expectedTribes := []db.Tribe{
			tribe,
			tribe2,
		}

		rctx := chi.NewRouteContext()
		req, err := http.NewRequestWithContext(context.WithValue(context.Background(), chi.RouteCtxKey, rctx), http.MethodGet, "/", nil)
		assert.NoError(t, err)

		handler.ServeHTTP(rr, req)
		var returnedTribes []db.Tribe
		err = json.Unmarshal(rr.Body.Bytes(), &returnedTribes)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rr.Code)
		assert.Len(t, returnedTribes, 2)
		assert.EqualValues(t, expectedTribes, returnedTribes)
	})
}

func TestGetTotalTribes(t *testing.T) {
	mockDb := mocks.NewDatabase(t)
	tHandler := NewTribeHandler(mockDb)
	t.Run("should return the total number of tribes", func(t *testing.T) {
		rr := httptest.NewRecorder()
		handler := http.HandlerFunc(tHandler.GetTotalribes)

		expectedTribes := []db.Tribe{
			{UUID: "uuid", Name: "Tribe1"},
			{UUID: "uuid", Name: "Tribe2"},
			{UUID: "uuid", Name: "Tribe3"},
		}

		expectedTribesCount := int64(len(expectedTribes))

		rctx := chi.NewRouteContext()
		req, err := http.NewRequestWithContext(context.WithValue(context.Background(), chi.RouteCtxKey, rctx), http.MethodGet, "/total", nil)
		assert.NoError(t, err)

		mockDb.On("GetTribesTotal", mock.Anything).Return(expectedTribesCount)

		handler.ServeHTTP(rr, req)
		var returnedTribesCount int64
		err = json.Unmarshal(rr.Body.Bytes(), &returnedTribesCount)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rr.Code)
		assert.EqualValues(t, expectedTribesCount, returnedTribesCount)
		mockDb.AssertExpectations(t)

	})
}

func TestGetListedTribes(t *testing.T) {
	mockDb := mocks.NewDatabase(t)
	tHandler := NewTribeHandler(mockDb)

	t.Run("should only return tribes associated with a passed tag query", func(t *testing.T) {
		rr := httptest.NewRecorder()
		handler := http.HandlerFunc(tHandler.GetListedTribes)
		expectedTribes := []db.Tribe{
			{UUID: "1", Name: "Tribe 1", Tags: pq.StringArray{"tag1", "tag2", "tag3"}},
			{UUID: "2", Name: "Tribe 2", Tags: pq.StringArray{"tag4", "tag5"}},
			{UUID: "3", Name: "Tribe 3", Tags: pq.StringArray{"tag6", "tag7", "tag8"}},
		}
		req, err := http.NewRequest("GET", "/tribes", nil)
		if err != nil {
			t.Fatal(err)
		}
		query := req.URL.Query()
		tagVals := pq.StringArray{"tag1", "tag4", "tag7"}
		tags := strings.Join(tagVals, ",")
		query.Set("tags", tags)
		req.URL.RawQuery = query.Encode()
		if err != nil {
			t.Fatal(err)
		}

		mockDb.On("GetListedTribes", req).Return(expectedTribes)
		handler.ServeHTTP(rr, req)
		var returnedTribes []db.Tribe
		err = json.Unmarshal(rr.Body.Bytes(), &returnedTribes)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rr.Code)
		assert.EqualValues(t, expectedTribes, returnedTribes)

	})

	t.Run("should return all tribes when no tag queries are passed", func(t *testing.T) {
		rr := httptest.NewRecorder()
		handler := http.HandlerFunc(tHandler.GetListedTribes)
		expectedTribes := []db.Tribe{
			{UUID: "1", Name: "Tribe 1", Tags: pq.StringArray{"tag1", "tag2", "tag3"}},
			{UUID: "2", Name: "Tribe 2", Tags: pq.StringArray{"tag4", "tag5"}},
			{UUID: "3", Name: "Tribe 3", Tags: pq.StringArray{"tag6", "tag7", "tag8"}},
		}

		req, err := http.NewRequest("GET", "/tribes", nil)
		if err != nil {
			t.Fatal(err)
		}
		query := req.URL.Query()
		tagVals := pq.StringArray{"tag1", "tag4", "tag7"}
		tags := strings.Join(tagVals, ",")
		query.Set("tags", tags)
		req.URL.RawQuery = query.Encode()
		if err != nil {
			t.Fatal(err)
		}

		mockDb.On("GetListedTribes", req).Return(expectedTribes)
		handler.ServeHTTP(rr, req)

		var returnedTribes []db.Tribe
		err = json.Unmarshal(rr.Body.Bytes(), &returnedTribes)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rr.Code)
		assert.EqualValues(t, expectedTribes, returnedTribes)

	})

}

func TestGenerateBudgetInvoice(t *testing.T) {
	ctx := context.Background()
	mockDb := mocks.NewDatabase(t)
	tHandler := NewTribeHandler(mockDb)
	authorizedCtx := context.WithValue(ctx, auth.ContextKey, "valid-key")

	userAmount := uint(1000)
	invoiceResponse := db.InvoiceResponse{
		Succcess: true,
		Response: db.Invoice{
			Invoice: "example_invoice",
		},
	}

	t.Run("Should test that a wrong Post body returns a 406 error", func(t *testing.T) {
		invalidBody := []byte(`"key": "value"`)
		req, err := http.NewRequestWithContext(authorizedCtx, http.MethodPost, "/budgetinvoices", bytes.NewBuffer(invalidBody))
		if err != nil {
			t.Fatal(err)
		}

		rr := httptest.NewRecorder()
		handler := http.HandlerFunc(tHandler.GenerateBudgetInvoice)
		handler.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusNotAcceptable, rr.Code)
	})

	t.Run("Should mock a call to relay /invoices with the correct body", func(t *testing.T) {
		mockDb.On("ProcessBudgetInvoice", mock.AnythingOfType("db.NewPaymentHistory"), mock.AnythingOfType("db.NewInvoiceList")).Return(nil)

		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

			expectedBody := map[string]interface{}{"amount": float64(0), "memo": "Budget Invoice"}
			var body map[string]interface{}
			err := json.NewDecoder(r.Body).Decode(&body)
			assert.NoError(t, err)

			assert.Equal(t, expectedBody, body)

			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{"result": "success"})
		}))
		defer ts.Close()

		config.RelayUrl = ts.URL

		reqBody := map[string]interface{}{"amount": 0}
		bodyBytes, _ := json.Marshal(reqBody)

		req, err := http.NewRequestWithContext(authorizedCtx, http.MethodPost, "/budgetinvoices", bytes.NewBuffer(bodyBytes))
		assert.NoError(t, err)

		rr := httptest.NewRecorder()
		handler := http.HandlerFunc(tHandler.GenerateBudgetInvoice)
		handler.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
	})

	t.Run("Should test that the amount passed by the user is equal to the amount sent for invoice generation", func(t *testing.T) {

		userAmount := float64(1000)

		mockDb.On("ProcessBudgetInvoice", mock.AnythingOfType("db.NewPaymentHistory"), mock.AnythingOfType("db.NewInvoiceList")).Return(nil)

		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var body map[string]interface{}
			err := json.NewDecoder(r.Body).Decode(&body)
			assert.NoError(t, err)

			assert.Equal(t, userAmount, body["amount"])

			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{"result": "success"})
		}))
		defer ts.Close()

		config.RelayUrl = ts.URL

		reqBody := map[string]interface{}{"amount": userAmount}
		bodyBytes, _ := json.Marshal(reqBody)

		req, err := http.NewRequestWithContext(authorizedCtx, http.MethodPost, "/budgetinvoices", bytes.NewBuffer(bodyBytes))
		assert.NoError(t, err)

		rr := httptest.NewRecorder()
		handler := http.HandlerFunc(tHandler.GenerateBudgetInvoice)
		handler.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
	})

	t.Run("Should add payments to the payment history and invoice to the invoice list upon successful relay call", func(t *testing.T) {
		mockDb.On("ProcessBudgetInvoice", mock.AnythingOfType("db.NewPaymentHistory"), mock.AnythingOfType("db.NewInvoiceList")).Return(nil)

		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(invoiceResponse)
		}))
		defer ts.Close()

		config.RelayUrl = ts.URL

		reqBody := map[string]interface{}{"amount": userAmount}
		bodyBytes, _ := json.Marshal(reqBody)
		req, err := http.NewRequestWithContext(authorizedCtx, http.MethodPost, "/budgetinvoices", bytes.NewBuffer(bodyBytes))
		assert.NoError(t, err)

		rr := httptest.NewRecorder()
		handler := http.HandlerFunc(tHandler.GenerateBudgetInvoice)
		handler.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)

		var response db.InvoiceResponse
		err = json.Unmarshal(rr.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.True(t, response.Succcess, "Invoice generation should be successful")
		assert.Equal(t, "example_invoice", response.Response.Invoice, "The invoice in the response should match the mock")

		mockDb.AssertCalled(t, "ProcessBudgetInvoice", mock.AnythingOfType("db.NewPaymentHistory"), mock.AnythingOfType("db.NewInvoiceList"))
	})
}
