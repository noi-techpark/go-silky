// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package silky

import (
	"context"
	"net/http"
	"testing"

	crawler_testing "github.com/noi-techpark/go-silky/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExampleForeachValue(t *testing.T) {
	mockTransport := crawler_testing.NewMockRoundTripper(map[string]string{
		"https://www.onecenter.info/api/DAZ/FacilityFreePlaces?FacilityID=1": "testdata/crawler/example_foreach_value/facilities_1.json",
		"https://www.onecenter.info/api/DAZ/FacilityFreePlaces?FacilityID=2": "testdata/crawler/example_foreach_value/facilities_2.json",
	})

	craw, _, _ := NewApiCrawler("testdata/crawler/example_foreach_value.yaml")
	client := &http.Client{Transport: mockTransport}
	craw.SetClient(client)

	err := craw.Run(context.TODO(), nil)
	require.Nil(t, err)

	data := craw.GetData()

	var expected interface{}
	err = crawler_testing.LoadInputData(&expected, "testdata/crawler/example_foreach_value/output.json")
	require.Nil(t, err)

	assert.Equal(t, expected, data)
}

func TestExampleForeachValueCtx(t *testing.T) {
	mockTransport := crawler_testing.NewMockRoundTripper(map[string]string{
		"https://www.onecenter.info/api/DAZ/FacilityFreePlaces?FacilityID=1": "testdata/crawler/example_foreach_value_transform_ctx/facilities_1.json",
		"https://www.onecenter.info/api/DAZ/FacilityFreePlaces?FacilityID=2": "testdata/crawler/example_foreach_value_transform_ctx/facilities_2.json",
	})

	craw, _, _ := NewApiCrawler("testdata/crawler/example_foreach_value_transform_ctx.yaml")
	client := &http.Client{Transport: mockTransport}
	craw.SetClient(client)

	err := craw.Run(context.TODO(), nil)
	require.Nil(t, err)

	data := craw.GetData()

	var expected interface{}
	err = crawler_testing.LoadInputData(&expected, "testdata/crawler/example_foreach_value_transform_ctx/output.json")
	require.Nil(t, err)

	assert.Equal(t, expected, data)
}

func TestExampleForeachValueStream(t *testing.T) {
	mockTransport := crawler_testing.NewMockRoundTripper(map[string]string{
		"https://www.onecenter.info/api/DAZ/FacilityFreePlaces?FacilityID=1": "testdata/crawler/example_foreach_value/facilities_1.json",
		"https://www.onecenter.info/api/DAZ/FacilityFreePlaces?FacilityID=2": "testdata/crawler/example_foreach_value/facilities_2.json",
	})

	craw, _, _ := NewApiCrawler("testdata/crawler/example_foreach_value_stream.yaml")
	client := &http.Client{Transport: mockTransport}
	craw.SetClient(client)

	stream := craw.GetDataStream()
	data := make([]interface{}, 0)

	// Use a channel to signal when goroutine is done collecting
	done := make(chan struct{})
	go func() {
		for d := range stream {
			data = append(data, d)
		}
		close(done)
	}()

	err := craw.Run(context.TODO(), nil)
	require.Nil(t, err)

	// Close stream to signal no more data, then wait for goroutine to finish
	close(stream)
	<-done

	var expected interface{}
	err = crawler_testing.LoadInputData(&expected, "testdata/crawler/example_foreach_value/output.json")
	require.Nil(t, err)

	assert.Equal(t, expected, data)
}

func TestExampleSingle(t *testing.T) {
	mockTransport := crawler_testing.NewMockRoundTripper(map[string]string{
		"https://www.onecenter.info/api/DAZ/GetFacilities":                   "testdata/crawler/example_single/facilities_1.json",
		"https://www.onecenter.info/api/DAZ/FacilityFreePlaces?FacilityID=2": "testdata/crawler/example_single/facility_id_2.json",
	})

	craw, _, _ := NewApiCrawler("testdata/crawler/example_single.yaml")
	client := &http.Client{Transport: mockTransport}
	craw.SetClient(client)

	err := craw.Run(context.TODO(), nil)
	require.Nil(t, err)

	data := craw.GetData()

	var expected interface{}
	err = crawler_testing.LoadInputData(&expected, "testdata/crawler/example_single/output.json")
	require.Nil(t, err)

	assert.Equal(t, expected, data)
}

func TestExample2(t *testing.T) {
	mockTransport := crawler_testing.NewMockRoundTripper(map[string]string{
		"https://www.onecenter.info/api/DAZ/GetFacilities":                    "testdata/crawler/example2/facilities_1.json",
		"https://www.onecenter.info/api/DAZ/FacilityFreePlaces?FacilityID=2":  "testdata/crawler/example2/facility_id_2.json",
		"https://www.onecenter.info/api/DAZ/FacilityFreePlaces?FacilityID=s3": "testdata/crawler/example2/facility_id_s3.json",
		"https://www.onecenter.info/api/DAZ/FacilityFreePlaces?FacilityID=s4": "testdata/crawler/example2/facility_id_s4.json",
		"https://www.onecenter.info/api/DAZ/Locations/l1":                     "testdata/crawler/example2/location_id_l1.json",
		"https://www.onecenter.info/api/DAZ/Locations/l2":                     "testdata/crawler/example2/location_id_l2.json",
		"https://www.onecenter.info/api/DAZ/Locations/l3":                     "testdata/crawler/example2/location_id_l3.json",
	})

	craw, _, _ := NewApiCrawler("testdata/crawler/example2.yaml")
	client := &http.Client{Transport: mockTransport}
	craw.SetClient(client)

	err := craw.Run(context.TODO(), nil)
	require.Nil(t, err)

	data := craw.GetData()

	var expected interface{}
	err = crawler_testing.LoadInputData(&expected, "testdata/crawler/example2/output.json")
	require.Nil(t, err)

	assert.Equal(t, expected, data)
}

func TestPaginatedIncrement(t *testing.T) {
	mockTransport := crawler_testing.NewMockRoundTripper(map[string]string{
		"https://www.onecenter.info/api/DAZ/GetFacilities?offset=0": "testdata/crawler/paginated_increment/facilities_1.json",
		"https://www.onecenter.info/api/DAZ/GetFacilities?offset=1": "testdata/crawler/paginated_increment/facilities_2.json",
	})

	craw, _, _ := NewApiCrawler("testdata/crawler/example_pagination_increment.yaml")
	client := &http.Client{Transport: mockTransport}
	craw.SetClient(client)

	err := craw.Run(context.TODO(), nil)
	require.Nil(t, err)

	data := craw.GetData()

	var expected interface{}
	err = crawler_testing.LoadInputData(&expected, "testdata/crawler/paginated_increment/output.json")
	require.Nil(t, err)

	assert.Equal(t, expected, data)
}

func TestPaginatedIncrementNested(t *testing.T) {
	mockTransport := crawler_testing.NewMockRoundTripper(map[string]string{
		"https://www.onecenter.info/api/DAZ/GetFacilities?offset=0":          "testdata/crawler/paginated_increment_stream/facilities_1.json",
		"https://www.onecenter.info/api/DAZ/GetFacilities?offset=1":          "testdata/crawler/paginated_increment_stream/facilities_2.json",
		"https://www.onecenter.info/api/DAZ/FacilityFreePlaces?FacilityID=1": "testdata/crawler/paginated_increment_stream/facility_id_1.json",
		"https://www.onecenter.info/api/DAZ/FacilityFreePlaces?FacilityID=2": "testdata/crawler/paginated_increment_stream/facility_id_2.json",
		"https://www.onecenter.info/api/DAZ/FacilityFreePlaces?FacilityID=3": "testdata/crawler/paginated_increment_stream/facility_id_3.json",
		"https://www.onecenter.info/api/DAZ/FacilityFreePlaces?FacilityID=4": "testdata/crawler/paginated_increment_stream/facility_id_4.json",
	})

	craw, _, _ := NewApiCrawler("testdata/crawler/example_pagination_increment_nested.yaml")
	client := &http.Client{Transport: mockTransport}
	craw.SetClient(client)

	err := craw.Run(context.TODO(), nil)
	require.Nil(t, err)

	data := craw.GetData()

	var expected interface{}
	err = crawler_testing.LoadInputData(&expected, "testdata/crawler/paginated_increment_stream/output.json")
	require.Nil(t, err)

	assert.Equal(t, expected, data)
}

func TestPaginatedIncrementStream(t *testing.T) {
	mockTransport := crawler_testing.NewMockRoundTripper(map[string]string{
		"https://www.onecenter.info/api/DAZ/GetFacilities?offset=0":          "testdata/crawler/paginated_increment_stream/facilities_1.json",
		"https://www.onecenter.info/api/DAZ/GetFacilities?offset=1":          "testdata/crawler/paginated_increment_stream/facilities_2.json",
		"https://www.onecenter.info/api/DAZ/FacilityFreePlaces?FacilityID=1": "testdata/crawler/paginated_increment_stream/facility_id_1.json",
		"https://www.onecenter.info/api/DAZ/FacilityFreePlaces?FacilityID=2": "testdata/crawler/paginated_increment_stream/facility_id_2.json",
		"https://www.onecenter.info/api/DAZ/FacilityFreePlaces?FacilityID=3": "testdata/crawler/paginated_increment_stream/facility_id_3.json",
		"https://www.onecenter.info/api/DAZ/FacilityFreePlaces?FacilityID=4": "testdata/crawler/paginated_increment_stream/facility_id_4.json",
	})

	craw, _, _ := NewApiCrawler("testdata/crawler/example_pagination_increment_stream.yaml")
	client := &http.Client{Transport: mockTransport}
	craw.SetClient(client)

	stream := craw.GetDataStream()
	data := make([]interface{}, 0)
	done := make(chan struct{})

	go func() {
		for d := range stream {
			data = append(data, d)
		}
		close(done)
	}()

	err := craw.Run(context.TODO(), nil)
	require.Nil(t, err)

	// Close the stream to signal the goroutine to finish (crawler doesn't close it)
	close(stream)

	// Wait for the goroutine to finish reading all stream data
	<-done

	var expected interface{}
	err = crawler_testing.LoadInputData(&expected, "testdata/crawler/paginated_increment_stream/output.json")
	require.Nil(t, err)

	assert.Equal(t, expected, data)
}

func TestPaginatedNextUrl(t *testing.T) {
	mockTransport := crawler_testing.NewMockRoundTripper(map[string]string{
		"https://www.onecenter.info/api/DAZ/GetFacilities": "testdata/crawler/next_url/facilities_1.json",
		"http://list.com/page2":                            "testdata/crawler/next_url/facilities_2.json",
	})

	craw, _, _ := NewApiCrawler("testdata/crawler/example_pagination_next.yaml")
	client := &http.Client{Transport: mockTransport}
	craw.SetClient(client)

	err := craw.Run(context.TODO(), nil)
	require.Nil(t, err)

	data := craw.GetData()

	var expected interface{}
	err = crawler_testing.LoadInputData(&expected, "testdata/crawler/next_url/output.json")
	require.Nil(t, err)

	assert.Equal(t, expected, data)
}

func TestParallelSimple(t *testing.T) {
	mockTransport := crawler_testing.NewMockRoundTripper(map[string]string{
		"https://api.example.com/items/1": "testdata/crawler/parallel/item_1.json",
		"https://api.example.com/items/2": "testdata/crawler/parallel/item_2.json",
		"https://api.example.com/items/3": "testdata/crawler/parallel/item_3.json",
		"https://api.example.com/items/4": "testdata/crawler/parallel/item_4.json",
		"https://api.example.com/items/5": "testdata/crawler/parallel/item_5.json",
	})

	craw, _, _ := NewApiCrawler("testdata/crawler/parallel/simple.yaml")
	client := &http.Client{Transport: mockTransport}
	craw.SetClient(client)

	err := craw.Run(context.TODO(), nil)
	require.Nil(t, err)

	data := craw.GetData()

	var expected interface{}
	err = crawler_testing.LoadInputData(&expected, "testdata/crawler/parallel/simple_output.json")
	require.Nil(t, err)

	assert.Equal(t, expected, data)
}

func TestParallelRateLimited(t *testing.T) {
	mockTransport := crawler_testing.NewMockRoundTripper(map[string]string{
		"https://api.example.com/items/1": "testdata/crawler/parallel/item_1.json",
		"https://api.example.com/items/2": "testdata/crawler/parallel/item_2.json",
		"https://api.example.com/items/3": "testdata/crawler/parallel/item_3.json",
	})

	craw, _, _ := NewApiCrawler("testdata/crawler/parallel/ratelimited.yaml")
	client := &http.Client{Transport: mockTransport}
	craw.SetClient(client)

	err := craw.Run(context.TODO(), nil)
	require.Nil(t, err)

	data := craw.GetData()

	var expected interface{}
	err = crawler_testing.LoadInputData(&expected, "testdata/crawler/parallel/ratelimited_output.json")
	require.Nil(t, err)

	assert.Equal(t, expected, data)
}

func TestParallelNoopMerge(t *testing.T) {
	mockTransport := crawler_testing.NewMockRoundTripper(map[string]string{
		"https://api.example.com/items/1": "testdata/crawler/parallel/item_1.json",
		"https://api.example.com/items/2": "testdata/crawler/parallel/item_2.json",
		"https://api.example.com/items/3": "testdata/crawler/parallel/item_3.json",
	})

	craw, _, _ := NewApiCrawler("testdata/crawler/parallel/noop_merge.yaml")
	client := &http.Client{Transport: mockTransport}
	craw.SetClient(client)

	err := craw.Run(context.TODO(), nil)
	require.Nil(t, err)

	data := craw.GetData()

	// Parallel execution doesn't guarantee order, so check for set equality
	resultMap, ok := data.(map[string]interface{})
	require.True(t, ok, "Result should be a map")

	items, ok := resultMap["items"].([]interface{})
	require.True(t, ok, "items should be an array")
	require.Len(t, items, 3, "Should have 3 items")

	// Check that all expected IDs are present
	ids := make(map[float64]bool)
	for _, item := range items {
		itemMap := item.(map[string]interface{})
		id := itemMap["id"].(float64)
		ids[id] = true
	}

	assert.True(t, ids[1], "Should contain item with id 1")
	assert.True(t, ids[2], "Should contain item with id 2")
	assert.True(t, ids[3], "Should contain item with id 3")
}

func TestParallelErrorHandling(t *testing.T) {
	mockTransport := crawler_testing.NewMockRoundTripper(map[string]string{
		"https://api.example.com/items/1": "testdata/crawler/parallel/item_1.json",
		"https://api.example.com/items/2": "testdata/crawler/parallel/invalid.json", // Invalid JSON will cause decode error
		"https://api.example.com/items/3": "testdata/crawler/parallel/item_3.json",
	})

	craw, _, _ := NewApiCrawler("testdata/crawler/parallel/error_handling.yaml")
	client := &http.Client{Transport: mockTransport}
	craw.SetClient(client)

	err := craw.Run(context.TODO(), nil)

	// Error should be propagated, not swallowed by goroutines
	require.NotNil(t, err, "Should return error when JSON decoding fails")
	assert.Contains(t, err.Error(), "error decoding response JSON", "Error should mention JSON decoding failure")
}

func TestParallelNestedParallelism(t *testing.T) {
	mockTransport := crawler_testing.NewMockRoundTripper(map[string]string{
		"https://api.example.com/categories/electronics/items": "testdata/crawler/parallel/category_electronics.json",
		"https://api.example.com/categories/books/items":       "testdata/crawler/parallel/category_books.json",
		"https://api.example.com/items/1":                      "testdata/crawler/parallel/item_detail_1.json",
		"https://api.example.com/items/2":                      "testdata/crawler/parallel/item_detail_2.json",
		"https://api.example.com/items/3":                      "testdata/crawler/parallel/item_detail_3.json",
		"https://api.example.com/items/4":                      "testdata/crawler/parallel/item_detail_4.json",
	})

	craw, _, _ := NewApiCrawler("testdata/crawler/parallel/nested_parallel.yaml")
	client := &http.Client{Transport: mockTransport}
	craw.SetClient(client)

	err := craw.Run(context.TODO(), nil)
	require.Nil(t, err)

	data := craw.GetData()

	// Should have 2 categories (electronics and books)
	resultArray, ok := data.([]interface{})
	require.True(t, ok, "Result should be an array")
	require.Len(t, resultArray, 2, "Should have 2 categories")

	// Each category should have items with details
	totalItems := 0
	for _, category := range resultArray {
		categoryArray, ok := category.([]interface{})
		require.True(t, ok, "Each category should be an array")
		totalItems += len(categoryArray)

		// Verify items have been enriched with details (price field)
		for _, item := range categoryArray {
			itemMap := item.(map[string]interface{})
			_, hasPrice := itemMap["price"]
			assert.True(t, hasPrice, "Item should have price from detail request")
		}
	}

	assert.Equal(t, 4, totalItems, "Should have 4 total items across both categories")
}

func TestParallelMultiRootParallel(t *testing.T) {
	mockTransport := crawler_testing.NewMockRoundTripper(map[string]string{
		"https://api.example.com/users/1":      "testdata/crawler/parallel/user_1.json",
		"https://api.example.com/users/2":      "testdata/crawler/parallel/user_2.json",
		"https://api.example.com/products/101": "testdata/crawler/parallel/product_101.json",
		"https://api.example.com/products/102": "testdata/crawler/parallel/product_102.json",
	})

	craw, _, _ := NewApiCrawler("testdata/crawler/parallel/multi_root_parallel.yaml")
	client := &http.Client{Transport: mockTransport}
	craw.SetClient(client)

	err := craw.Run(context.TODO(), nil)
	require.Nil(t, err)

	data := craw.GetData()

	// Check result structure
	resultMap, ok := data.(map[string]interface{})
	require.True(t, ok, "Result should be a map")

	users, ok := resultMap["users"].([]interface{})
	require.True(t, ok, "users should be an array")
	require.Len(t, users, 2, "Should have 2 users")

	products, ok := resultMap["products"].([]interface{})
	require.True(t, ok, "products should be an array")
	require.Len(t, products, 2, "Should have 2 products")

	// Verify users have correct IDs
	userIds := make(map[float64]bool)
	for _, user := range users {
		userMap := user.(map[string]interface{})
		id := userMap["id"].(float64)
		userIds[id] = true
	}
	assert.True(t, userIds[1], "Should have user with id 1")
	assert.True(t, userIds[2], "Should have user with id 2")

	// Verify products have correct IDs
	productIds := make(map[float64]bool)
	for _, product := range products {
		productMap := product.(map[string]interface{})
		id := productMap["id"].(float64)
		productIds[id] = true
	}
	assert.True(t, productIds[101], "Should have product with id 101")
	assert.True(t, productIds[102], "Should have product with id 102")
}

func TestPostJSONBody(t *testing.T) {
	mockTransport, err := crawler_testing.NewMockRoundTripperFromYAML("testdata/crawler/post_json_body/mocks.yaml")
	require.Nil(t, err)

	craw, _, _ := NewApiCrawler("testdata/crawler/post_json_body.yaml")
	client := &http.Client{Transport: mockTransport}
	craw.SetClient(client)

	err = craw.Run(context.TODO(), nil)
	require.Nil(t, err)

	// Check for validation errors
	validationErrors := mockTransport.GetErrors()
	if len(validationErrors) > 0 {
		t.Fatalf("Mock validation failed: %v", validationErrors)
	}

	data := craw.GetData()

	var expected interface{}
	err = crawler_testing.LoadInputData(&expected, "testdata/crawler/post_json_body/output.json")
	require.Nil(t, err)

	assert.Equal(t, expected, data)
}

func TestPostFormURLEncoded(t *testing.T) {
	mockTransport, err := crawler_testing.NewMockRoundTripperFromYAML("testdata/crawler/post_form_urlencoded/mocks.yaml")
	require.Nil(t, err)

	craw, _, _ := NewApiCrawler("testdata/crawler/post_form_urlencoded.yaml")
	client := &http.Client{Transport: mockTransport}
	craw.SetClient(client)

	err = craw.Run(context.TODO(), nil)
	require.Nil(t, err)

	// Check for validation errors
	validationErrors := mockTransport.GetErrors()
	if len(validationErrors) > 0 {
		t.Fatalf("Mock validation failed: %v", validationErrors)
	}

	data := craw.GetData()

	var expected interface{}
	err = crawler_testing.LoadInputData(&expected, "testdata/crawler/post_form_urlencoded/output.json")
	require.Nil(t, err)

	assert.Equal(t, expected, data)
}

func TestPostBodyMergePagination(t *testing.T) {
	mockTransport, err := crawler_testing.NewMockRoundTripperFromYAML("testdata/crawler/post_body_merge_pagination/mocks.yaml")
	require.Nil(t, err)

	craw, validationErrors, err := NewApiCrawler("testdata/crawler/post_body_merge_pagination.yaml")
	if err != nil {
		t.Fatalf("Failed to create crawler: %v, validation errors: %v", err, validationErrors)
	}
	require.Nil(t, err)
	require.Empty(t, validationErrors)

	client := &http.Client{Transport: mockTransport}
	craw.SetClient(client)

	err = craw.Run(context.TODO(), nil)
	require.Nil(t, err)

	// Check for validation errors
	mockValidationErrors := mockTransport.GetErrors()
	if len(mockValidationErrors) > 0 {
		t.Fatalf("Mock validation failed: %v", mockValidationErrors)
	}

	data := craw.GetData()

	// Check that results were merged correctly
	resultMap, ok := data.(map[string]interface{})
	require.True(t, ok, "Result should be a map")

	results, ok := resultMap["results"].([]interface{})
	require.True(t, ok, "results should be an array")
	require.Equal(t, 4, len(results), "Should have exactly 4 results (2 pages)")
}

// Authentication Tests

func TestAuthBasic(t *testing.T) {
	mockTransport := crawler_testing.NewMockRoundTripper(map[string]string{
		"https://api.example.com/v1/users": "testdata/crawler/auth_basic/users_response.json",
	})

	craw, _, _ := NewApiCrawler("testdata/crawler/auth_basic.yaml")
	client := &http.Client{Transport: mockTransport}
	craw.SetClient(client)

	err := craw.Run(context.TODO(), nil)
	require.Nil(t, err)

	data := craw.GetData()
	resultArray, ok := data.([]interface{})
	require.True(t, ok, "Result should be an array")
	require.Greater(t, len(resultArray), 0, "Should have users")
}

func TestAuthBearer(t *testing.T) {
	mockTransport := crawler_testing.NewMockRoundTripper(map[string]string{
		"https://api.example.com/v2/resources": "testdata/crawler/auth_bearer/resources_response.json",
	})

	craw, _, _ := NewApiCrawler("testdata/crawler/auth_bearer.yaml")
	client := &http.Client{Transport: mockTransport}
	craw.SetClient(client)

	err := craw.Run(context.TODO(), nil)
	require.Nil(t, err)

	data := craw.GetData()
	resultMap, ok := data.(map[string]interface{})
	require.True(t, ok, "Result should be a map")
	require.NotNil(t, resultMap, "Should have resources data")
}

func TestAuthOAuthPassword(t *testing.T) {
	mockTransport := crawler_testing.NewMockRoundTripper(map[string]string{
		"https://oauth.example.com/token":    "testdata/crawler/auth_oauth_password/token_response.json",
		"https://api.example.com/me/profile": "testdata/crawler/auth_oauth_password/profile_response.json",
	})

	craw, _, _ := NewApiCrawler("testdata/crawler/auth_oauth_password.yaml")
	client := &http.Client{Transport: mockTransport}
	craw.SetClient(client)

	err := craw.Run(context.TODO(), nil)
	require.Nil(t, err)

	data := craw.GetData()
	resultArray, ok := data.([]interface{})
	require.True(t, ok, "Result should be an array")
	require.Greater(t, len(resultArray), 0, "Should have profile data")
}

func TestAuthOAuthClientCredentials(t *testing.T) {
	mockTransport := crawler_testing.NewMockRoundTripper(map[string]string{
		"https://oauth.example.com/v2/token":     "testdata/crawler/auth_oauth_client_credentials/token_response.json",
		"https://api.example.com/admin/api-keys": "testdata/crawler/auth_oauth_client_credentials/api_keys_response.json",
	})

	craw, _, _ := NewApiCrawler("testdata/crawler/auth_oauth_client_credentials.yaml")
	client := &http.Client{Transport: mockTransport}
	craw.SetClient(client)

	err := craw.Run(context.TODO(), nil)
	require.Nil(t, err)

	data := craw.GetData()
	resultMap, ok := data.(map[string]interface{})
	require.True(t, ok, "Result should be a map")
	require.NotNil(t, resultMap, "Should have API keys data")
}

func TestAuthCookie(t *testing.T) {
	mockTransport := crawler_testing.NewMockRoundTripper(map[string]string{
		"https://app.example.com/api/auth/login": "testdata/crawler/auth_cookie/login_response.json",
		"https://app.example.com/api/dashboard":  "testdata/crawler/auth_cookie/dashboard_response.json",
	})

	// Mock needs to set the session_token cookie
	mockTransport.InterceptFunc = func(req *http.Request, resp *http.Response) {
		if req.URL.Path == "/api/auth/login" {
			cookie := &http.Cookie{
				Name:  "session_token",
				Value: "mock_session_abc123",
			}
			resp.Header.Add("Set-Cookie", cookie.String())
		}
	}

	craw, _, _ := NewApiCrawler("testdata/crawler/auth_cookie.yaml")
	client := &http.Client{Transport: mockTransport}
	craw.SetClient(client)

	err := craw.Run(context.TODO(), nil)
	require.Nil(t, err)

	data := craw.GetData()
	resultArray, ok := data.([]interface{})
	require.True(t, ok, "Result should be an array")
	require.Greater(t, len(resultArray), 0, "Should have dashboard widgets")
}

func TestAuthJWTBody(t *testing.T) {
	mockTransport := crawler_testing.NewMockRoundTripper(map[string]string{
		"https://api.example.com/v1/auth/login":     "testdata/crawler/auth_jwt_body/login_response.json",
		"https://api.example.com/v1/protected/data": "testdata/crawler/auth_jwt_body/protected_response.json",
	})

	craw, _, _ := NewApiCrawler("testdata/crawler/auth_jwt_body.yaml")
	client := &http.Client{Transport: mockTransport}
	craw.SetClient(client)

	err := craw.Run(context.TODO(), nil)
	require.Nil(t, err)

	data := craw.GetData()
	resultMap, ok := data.(map[string]interface{})
	require.True(t, ok, "Result should be a map")
	require.NotNil(t, resultMap, "Should have protected data")
}

func TestAuthJWTHeader(t *testing.T) {
	mockTransport := crawler_testing.NewMockRoundTripper(map[string]string{
		"https://api.example.com/auth/signin": "testdata/crawler/auth_jwt_header/login_response.json",
		"https://api.example.com/documents":   "testdata/crawler/auth_jwt_header/documents_response.json",
	})

	// Mock needs to set the X-Auth-Token header
	mockTransport.InterceptFunc = func(req *http.Request, resp *http.Response) {
		if req.URL.Path == "/auth/signin" {
			resp.Header.Set("X-Auth-Token", "mock_jwt_token_xyz789")
		}
	}

	craw, _, _ := NewApiCrawler("testdata/crawler/auth_jwt_header.yaml")
	client := &http.Client{Transport: mockTransport}
	craw.SetClient(client)

	err := craw.Run(context.TODO(), nil)
	require.Nil(t, err)

	data := craw.GetData()
	resultArray, ok := data.([]interface{})
	require.True(t, ok, "Result should be an array")
	require.Greater(t, len(resultArray), 0, "Should have documents")
}

func TestAuthCustomCookieToHeader(t *testing.T) {
	mockTransport := crawler_testing.NewMockRoundTripper(map[string]string{
		"https://legacy.example.com/login":       "testdata/crawler/auth_custom_cookie_to_header/login_response.json",
		"https://legacy.example.com/api/systems": "testdata/crawler/auth_custom_cookie_to_header/systems_response.json",
	})

	// Mock needs to set the auth_session cookie
	mockTransport.InterceptFunc = func(req *http.Request, resp *http.Response) {
		if req.URL.Path == "/login" {
			cookie := &http.Cookie{
				Name:  "auth_session",
				Value: "mock_auth_session_456",
			}
			resp.Header.Add("Set-Cookie", cookie.String())
		}
	}

	craw, _, _ := NewApiCrawler("testdata/crawler/auth_custom_cookie_to_header.yaml")
	client := &http.Client{Transport: mockTransport}
	craw.SetClient(client)

	err := craw.Run(context.TODO(), nil)
	require.Nil(t, err)

	data := craw.GetData()
	resultMap, ok := data.(map[string]interface{})
	require.True(t, ok, "Result should be a map")
	require.NotNil(t, resultMap, "Should have systems data")
}

func TestAuthCustomBodyToQuery(t *testing.T) {
	mockTransport := crawler_testing.NewMockRoundTripper(map[string]string{
		"https://api.provider.com/v1/authenticate":                                        "testdata/crawler/auth_custom_body_to_query/authenticate_response.json",
		"https://api.provider.com/v1/sensors?api_key=ak_live_1234567890abcdef":            "testdata/crawler/auth_custom_body_to_query/sensors_response.json",
		"https://api.provider.com/v1/sensors/1/readings?api_key=ak_live_1234567890abcdef": "testdata/crawler/auth_custom_body_to_query/readings_response.json",
		"https://api.provider.com/v1/sensors/2/readings?api_key=ak_live_1234567890abcdef": "testdata/crawler/auth_custom_body_to_query/readings_response.json",
	})

	craw, _, _ := NewApiCrawler("testdata/crawler/auth_custom_body_to_query.yaml")
	client := &http.Client{Transport: mockTransport}
	craw.SetClient(client)

	err := craw.Run(context.TODO(), nil)
	require.Nil(t, err)

	data := craw.GetData()
	resultArray, ok := data.([]interface{})
	require.True(t, ok, "Result should be an array")
	require.Greater(t, len(resultArray), 0, "Should have sensors with readings")
}

func TestAuthMixedOverride(t *testing.T) {
	mockTransport := crawler_testing.NewMockRoundTripper(map[string]string{
		"https://api.example.com/public/stats":       "testdata/crawler/auth_mixed_override/stats_response.json",
		"https://admin.example.com/internal/reports": "testdata/crawler/auth_mixed_override/reports_response.json",
	})

	craw, _, _ := NewApiCrawler("testdata/crawler/auth_mixed_override.yaml")
	client := &http.Client{Transport: mockTransport}
	craw.SetClient(client)

	err := craw.Run(context.TODO(), nil)
	require.Nil(t, err)

	data := craw.GetData()
	resultMap, ok := data.(map[string]interface{})
	require.True(t, ok, "Result should be a map")
	require.NotNil(t, resultMap["stats"], "Should have stats")
	require.NotNil(t, resultMap["reports"], "Should have reports")
}

// Request "as" Property Tests

func TestForValuesContextPreservation(t *testing.T) {
	mockTransport := crawler_testing.NewMockRoundTripper(map[string]string{
		"https://api.store.com/v1/categories?type=electronics":                 "testdata/crawler/request_as_context_disconnect/categories_response.json",
		"https://api.store.com/v1/categories?type=clothing":                    "testdata/crawler/request_as_context_disconnect/categories_clothing_response.json",
		"https://api.store.com/v1/products?type=electronics&categoryId=cat-e1": "testdata/crawler/request_as_context_disconnect/products_response.json",
		"https://api.store.com/v1/products?type=electronics&categoryId=cat-e2": "testdata/crawler/request_as_context_disconnect/products_response.json",
		"https://api.store.com/v1/products?type=clothing&categoryId=cat-c1":    "testdata/crawler/request_as_context_disconnect/products_response.json",
		"https://api.store.com/v1/products?type=clothing&categoryId=cat-c2":    "testdata/crawler/request_as_context_disconnect/products_response.json",
	})

	craw, validationErrors, err := NewApiCrawler("testdata/crawler/request_as_context_disconnect.yaml")
	if err != nil {
		for _, ve := range validationErrors {
			t.Logf("Validation error: %v", ve)
		}
	}
	require.Nil(t, err, "Failed to load crawler config")
	client := &http.Client{Transport: mockTransport}
	craw.SetClient(client)

	err = craw.Run(context.TODO(), nil)
	require.Nil(t, err)

	data := craw.GetData()
	resultArray, ok := data.([]interface{})
	require.True(t, ok, "Result should be an array")
	require.Greater(t, len(resultArray), 0, "Should have products")

	// Verify products have both categoryType and categoryName (from preserved contexts)
	for _, item := range resultArray {
		productMap := item.(map[string]interface{})
		require.NotNil(t, productMap["categoryType"], "Product should have categoryType")
		require.NotNil(t, productMap["categoryName"], "Product should have categoryName")
	}
}

func TestForValuesDynamicKeys(t *testing.T) {
	mockTransport := crawler_testing.NewMockRoundTripper(map[string]string{
		"https://api.locations.com/v1/locations?lang=en":                 "testdata/crawler/request_as_dynamic_keys/locations_en_response.json",
		"https://api.locations.com/v1/locations?lang=de":                 "testdata/crawler/request_as_dynamic_keys/locations_de_response.json",
		"https://api.locations.com/v1/locations?lang=it":                 "testdata/crawler/request_as_dynamic_keys/locations_it_response.json",
		"https://api.locations.com/v1/stations?lang=en&locationId=loc-1": "testdata/crawler/request_as_dynamic_keys/stations_response.json",
		"https://api.locations.com/v1/stations?lang=en&locationId=loc-2": "testdata/crawler/request_as_dynamic_keys/stations_response.json",
		"https://api.locations.com/v1/stations?lang=de&locationId=loc-1": "testdata/crawler/request_as_dynamic_keys/stations_response.json",
		"https://api.locations.com/v1/stations?lang=de&locationId=loc-2": "testdata/crawler/request_as_dynamic_keys/stations_response.json",
		"https://api.locations.com/v1/stations?lang=it&locationId=loc-1": "testdata/crawler/request_as_dynamic_keys/stations_response.json",
		"https://api.locations.com/v1/stations?lang=it&locationId=loc-2": "testdata/crawler/request_as_dynamic_keys/stations_response.json",
		"https://api.locations.com/v1/users/me/preferences":              "testdata/crawler/request_as_dynamic_keys/preferences_response.json",
	})

	craw, _, err := NewApiCrawler("testdata/crawler/request_as_dynamic_keys.yaml")
	require.Nil(t, err, "Failed to load crawler config")
	client := &http.Client{Transport: mockTransport}
	craw.SetClient(client)

	err = craw.Run(context.TODO(), nil)
	require.Nil(t, err)

	data := craw.GetData()
	resultMap, ok := data.(map[string]interface{})
	require.True(t, ok, "Result should be a map")

	// Verify dynamic keys were created (en, de, it)
	require.NotNil(t, resultMap["en"], "Should have 'en' key")
	require.NotNil(t, resultMap["de"], "Should have 'de' key")
	require.NotNil(t, resultMap["it"], "Should have 'it' key")

	// Verify each language has locations with stations
	enLocations, ok := resultMap["en"].([]interface{})
	require.True(t, ok, "en should be an array")
	require.Greater(t, len(enLocations), 0, "Should have English locations")

	// Verify stations were added (from nested requests that accessed .language)
	for _, loc := range enLocations {
		locMap := loc.(map[string]interface{})
		require.NotNil(t, locMap["stations"], "Location should have stations")
	}

	// Verify preferences were added with availableLanguages
	require.NotNil(t, resultMap["availableLanguages"], "Should have availableLanguages")
}

// ForValues Tests

func TestForValuesSimple(t *testing.T) {
	mockTransport := crawler_testing.NewMockRoundTripper(map[string]string{
		"https://api.example.com/translations?lang=en": "testdata/crawler/forvalues_simple/translations_en.json",
		"https://api.example.com/translations?lang=de": "testdata/crawler/forvalues_simple/translations_de.json",
		"https://api.example.com/translations?lang=it": "testdata/crawler/forvalues_simple/translations_it.json",
	})

	craw, _, err := NewApiCrawler("testdata/crawler/forvalues_simple.yaml")
	require.Nil(t, err, "Failed to load crawler config")
	client := &http.Client{Transport: mockTransport}
	craw.SetClient(client)

	err = craw.Run(context.TODO(), nil)
	require.Nil(t, err)

	data := craw.GetData()
	resultMap, ok := data.(map[string]interface{})
	require.True(t, ok, "Result should be a map")

	// Verify dynamic keys were created
	require.NotNil(t, resultMap["en"], "Should have 'en' key")
	require.NotNil(t, resultMap["de"], "Should have 'de' key")
	require.NotNil(t, resultMap["it"], "Should have 'it' key")

	// Verify translations content
	enTranslations := resultMap["en"].(map[string]interface{})
	require.Equal(t, "Hello", enTranslations["hello"])
	require.Equal(t, "Goodbye", enTranslations["goodbye"])

	deTranslations := resultMap["de"].(map[string]interface{})
	require.Equal(t, "Hallo", deTranslations["hello"])
}

func TestForValuesNested(t *testing.T) {
	mockTransport := crawler_testing.NewMockRoundTripper(map[string]string{
		"https://api.example.com/config?region=eu&env=prod":    "testdata/crawler/forvalues_nested/config_eu_prod.json",
		"https://api.example.com/config?region=eu&env=staging": "testdata/crawler/forvalues_nested/config_eu_staging.json",
		"https://api.example.com/config?region=us&env=prod":    "testdata/crawler/forvalues_nested/config_us_prod.json",
		"https://api.example.com/config?region=us&env=staging": "testdata/crawler/forvalues_nested/config_us_staging.json",
	})

	craw, _, err := NewApiCrawler("testdata/crawler/forvalues_nested.yaml")
	require.Nil(t, err, "Failed to load crawler config")
	client := &http.Client{Transport: mockTransport}
	craw.SetClient(client)

	err = craw.Run(context.TODO(), nil)
	require.Nil(t, err)

	data := craw.GetData()
	resultMap, ok := data.(map[string]interface{})
	require.True(t, ok, "Result should be a map")

	// Verify nested structure
	require.NotNil(t, resultMap["eu"], "Should have 'eu' region")
	require.NotNil(t, resultMap["us"], "Should have 'us' region")

	euMap := resultMap["eu"].(map[string]interface{})
	require.NotNil(t, euMap["prod"], "eu should have 'prod' env")
	require.NotNil(t, euMap["staging"], "eu should have 'staging' env")

	// Verify config values
	euProd := euMap["prod"].(map[string]interface{})
	require.Equal(t, "eu", euProd["region"])
	require.Equal(t, "prod", euProd["env"])
	require.Equal(t, float64(100), euProd["maxConnections"])
}

func TestForValuesWithObjects(t *testing.T) {
	mockTransport := crawler_testing.NewMockRoundTripper(map[string]string{
		"https://api.example.com/languages/en": "testdata/crawler/forvalues_objects/lang_en.json",
		"https://api.example.com/languages/de": "testdata/crawler/forvalues_objects/lang_de.json",
		"https://api.example.com/languages/it": "testdata/crawler/forvalues_objects/lang_it.json",
	})

	craw, _, err := NewApiCrawler("testdata/crawler/forvalues_objects.yaml")
	require.Nil(t, err, "Failed to load crawler config")
	client := &http.Client{Transport: mockTransport}
	craw.SetClient(client)

	err = craw.Run(context.TODO(), nil)
	require.Nil(t, err)

	data := craw.GetData()
	resultArray, ok := data.([]interface{})
	require.True(t, ok, "Result should be an array")
	require.Equal(t, 3, len(resultArray), "Should have 3 language entries")

	// Verify first entry has object properties merged
	first := resultArray[0].(map[string]interface{})
	require.NotNil(t, first["code"], "Should have code from object")
	require.NotNil(t, first["name"], "Should have name from object")
	require.NotNil(t, first["data"], "Should have data from response")
}

// Edge Case Tests for Context Cloning

func TestEdgeCaseDeepNesting(t *testing.T) {
	// Tests that deeply nested forValues can all merge to the canonical root context
	// without the working contexts interfering
	mockTransport := crawler_testing.NewMockRoundTripper(map[string]string{
		"https://api.example.com/level3?l1=L1&l2=L2": "testdata/crawler/edge_case_deep_nesting/level3.json",
	})

	craw, validationErrors, err := NewApiCrawler("testdata/crawler/edge_case_deep_nesting.yaml")
	if err != nil {
		for _, ve := range validationErrors {
			t.Logf("Validation error: %v", ve)
		}
	}
	require.Nil(t, err, "Failed to load crawler config")
	client := &http.Client{Transport: mockTransport}
	craw.SetClient(client)

	err = craw.Run(context.TODO(), nil)
	require.Nil(t, err)

	data := craw.GetData()
	resultMap, ok := data.(map[string]interface{})
	require.True(t, ok, "Result should be a map")

	items, ok := resultMap["items"].([]interface{})
	require.True(t, ok, "items should be an array")
	require.Equal(t, 1, len(items), "Should have one item merged from deep nesting")

	// Verify the deeply nested result has all context values preserved
	item := items[0].(map[string]interface{})
	require.Equal(t, "L1", item["level1Id"], "Should have level1Id from context")
	require.Equal(t, "L2", item["level2Id"], "Should have level2Id from context")
	require.NotNil(t, item["level3Data"], "Should have level3Data from response")
}

func TestEdgeCaseMultipleForValues(t *testing.T) {
	// Tests that multiple forValues iterations can all merge to canonical root
	// without context shadowing issues
	mockTransport := crawler_testing.NewMockRoundTripper(map[string]string{
		"https://api.example.com/data?region=us&tier=free":    "testdata/crawler/edge_case_multiple_forvalues/response.json",
		"https://api.example.com/data?region=us&tier=premium": "testdata/crawler/edge_case_multiple_forvalues/response.json",
		"https://api.example.com/data?region=eu&tier=free":    "testdata/crawler/edge_case_multiple_forvalues/response.json",
		"https://api.example.com/data?region=eu&tier=premium": "testdata/crawler/edge_case_multiple_forvalues/response.json",
	})

	craw, validationErrors, err := NewApiCrawler("testdata/crawler/edge_case_multiple_forvalues.yaml")
	if err != nil {
		for _, ve := range validationErrors {
			t.Logf("Validation error: %v", ve)
		}
	}
	require.Nil(t, err, "Failed to load crawler config")
	client := &http.Client{Transport: mockTransport}
	craw.SetClient(client)

	err = craw.Run(context.TODO(), nil)
	require.Nil(t, err)

	data := craw.GetData()
	resultMap, ok := data.(map[string]interface{})
	require.True(t, ok, "Result should be a map")

	results, ok := resultMap["results"].([]interface{})
	require.True(t, ok, "results should be an array")
	require.Equal(t, 4, len(results), "Should have 4 items (2 regions x 2 tiers)")

	// Verify each result has correct context values
	regions := make(map[string]int)
	tiers := make(map[string]int)

	for _, r := range results {
		result := r.(map[string]interface{})
		require.NotNil(t, result["region"], "Should have region")
		require.NotNil(t, result["tier"], "Should have tier")
		require.NotNil(t, result["data"], "Should have data")

		regions[result["region"].(string)]++
		tiers[result["tier"].(string)]++
	}

	// Each region should appear twice (once per tier)
	require.Equal(t, 2, regions["us"], "us should appear twice")
	require.Equal(t, 2, regions["eu"], "eu should appear twice")

	// Each tier should appear twice (once per region)
	require.Equal(t, 2, tiers["free"], "free should appear twice")
	require.Equal(t, 2, tiers["premium"], "premium should appear twice")
}

// Nested Merge Tests - verify deep nesting with different merge targets

func TestNestedMergeToCurrentContext(t *testing.T) {
	// Tests nested forEach with mergeWithContext to station context
	// Each station gets enriched with places from the detail request
	mockTransport := crawler_testing.NewMockRoundTripper(map[string]string{
		"https://api.example.com/locations":                "testdata/crawler/nested_merge/locations.json",
		"https://api.example.com/stations?locationId=loc1": "testdata/crawler/nested_merge/stations_loc1.json",
		"https://api.example.com/stations?locationId=loc2": "testdata/crawler/nested_merge/stations_loc2.json",
		"https://api.example.com/station?stationID=sta1":   "testdata/crawler/nested_merge/station_sta1.json",
		"https://api.example.com/station?stationID=sta2":   "testdata/crawler/nested_merge/station_sta2.json",
		"https://api.example.com/station?stationID=sta3":   "testdata/crawler/nested_merge/station_sta3.json",
	})

	craw, validationErrors, err := NewApiCrawler("testdata/crawler/nested_merge/config_current.yaml")
	if err != nil {
		for _, ve := range validationErrors {
			t.Logf("Validation error: %v", ve)
		}
	}
	require.Nil(t, err, "Failed to load crawler config")
	client := &http.Client{Transport: mockTransport}
	craw.SetClient(client)

	err = craw.Run(context.TODO(), nil)
	require.Nil(t, err)

	data := craw.GetData()

	var expected interface{}
	err = crawler_testing.LoadInputData(&expected, "testdata/crawler/nested_merge/output_current.json")
	require.Nil(t, err)

	assert.Equal(t, expected, data)
}

func TestNestedMergeToAncestorContext(t *testing.T) {
	// Tests mergeWithContext to ancestor context (location, not station)
	// All places from all stations are collected into the location's allPlaces array
	// This verifies we're not shadowing ancestor contexts
	mockTransport := crawler_testing.NewMockRoundTripper(map[string]string{
		"https://api.example.com/locations":                "testdata/crawler/nested_merge/locations.json",
		"https://api.example.com/stations?locationId=loc1": "testdata/crawler/nested_merge/stations_loc1.json",
		"https://api.example.com/stations?locationId=loc2": "testdata/crawler/nested_merge/stations_loc2.json",
		"https://api.example.com/station?stationID=sta1":   "testdata/crawler/nested_merge/station_sta1.json",
		"https://api.example.com/station?stationID=sta2":   "testdata/crawler/nested_merge/station_sta2.json",
		"https://api.example.com/station?stationID=sta3":   "testdata/crawler/nested_merge/station_sta3.json",
	})

	craw, validationErrors, err := NewApiCrawler("testdata/crawler/nested_merge/config_ancestor.yaml")
	if err != nil {
		for _, ve := range validationErrors {
			t.Logf("Validation error: %v", ve)
		}
	}
	require.Nil(t, err, "Failed to load crawler config")
	client := &http.Client{Transport: mockTransport}
	craw.SetClient(client)

	err = craw.Run(context.TODO(), nil)
	require.Nil(t, err)

	data := craw.GetData()

	var expected interface{}
	err = crawler_testing.LoadInputData(&expected, "testdata/crawler/nested_merge/output_ancestor.json")
	require.Nil(t, err)

	assert.Equal(t, expected, data)
}

func TestForEachNilItemSkip(t *testing.T) {
	mockTransport := crawler_testing.NewMockRoundTripper(map[string]string{
		"https://api.example.com/resorts":        "testdata/crawler/foreach_nil_skip/resorts.json",
		"https://api.example.com/slopes/slope1":  "testdata/crawler/foreach_nil_skip/slope_slope1.json",
		"https://api.example.com/slopes/slope2":  "testdata/crawler/foreach_nil_skip/slope_slope2.json",
	})

	craw, validationErrors, err := NewApiCrawler("testdata/crawler/foreach_nil_skip.yaml")
	if err != nil {
		for _, ve := range validationErrors {
			t.Logf("Validation error: %v", ve)
		}
	}
	require.Nil(t, err, "Failed to load crawler config")
	client := &http.Client{Transport: mockTransport}
	craw.SetClient(client)

	err = craw.Run(context.TODO(), nil)
	require.Nil(t, err, "Run should not panic or error on nil forEach items")

	data := craw.GetData()

	var expected interface{}
	err = crawler_testing.LoadInputData(&expected, "testdata/crawler/foreach_nil_skip/output.json")
	require.Nil(t, err)

	assert.Equal(t, expected, data)
}

func TestStreamingForValues(t *testing.T) {
	// Tests streaming with forValues - each iteration emits a structure
	mockTransport := crawler_testing.NewMockRoundTripper(map[string]string{
		"https://api.example.com/forecasts/id-001.json": "testdata/crawler/stream_forvalues/forecast_id1.json",
		"https://api.example.com/forecasts/id-002.json": "testdata/crawler/stream_forvalues/forecast_id2.json",
		"https://api.example.com/forecasts/id-003.json": "testdata/crawler/stream_forvalues/forecast_id3.json",
	})

	craw, validationErrors, err := NewApiCrawler("testdata/crawler/stream_forvalues/config.yaml")
	if err != nil {
		for _, ve := range validationErrors {
			t.Logf("Validation error: %v", ve)
		}
	}
	require.Nil(t, err, "Failed to load crawler config")
	client := &http.Client{Transport: mockTransport}
	craw.SetClient(client)

	stream := craw.GetDataStream()
	var streamedItems []interface{}

	// Collect streamed items
	done := make(chan struct{})
	go func() {
		for item := range stream {
			streamedItems = append(streamedItems, item)
		}
		close(done)
	}()

	err = craw.Run(context.TODO(), nil)
	require.Nil(t, err)

	// Close stream to signal no more data, then wait for goroutine to finish
	close(stream)
	<-done

	// Should have streamed 3 items (one per forValues iteration)
	require.Equal(t, 3, len(streamedItems), "Should stream 3 items")

	// Verify each streamed item has the expected structure
	for i, item := range streamedItems {
		itemMap, ok := item.(map[string]interface{})
		require.True(t, ok, "Streamed item %d should be a map", i)
		require.NotNil(t, itemMap["id"], "Item %d should have 'id'", i)
		require.NotNil(t, itemMap["forecast"], "Item %d should have 'forecast'", i)

		// Verify forecast has expected fields
		forecast, ok := itemMap["forecast"].(map[string]interface{})
		require.True(t, ok, "forecast should be a map")
		require.NotNil(t, forecast["temperature"], "forecast should have temperature")
		require.NotNil(t, forecast["conditions"], "forecast should have conditions")
	}
}
