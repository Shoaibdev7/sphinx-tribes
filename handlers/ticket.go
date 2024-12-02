package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/go-chi/chi"
	"github.com/google/uuid"
	"github.com/stakwork/sphinx-tribes/auth"
	"github.com/stakwork/sphinx-tribes/db"
	"github.com/stakwork/sphinx-tribes/utils"
)

type ticketHandler struct {
	httpClient HttpClient
	db         db.Database
}

type TicketResponse struct {
	Success  bool     `json:"success"`
	TicketID string   `json:"ticket_id,omitempty"`
	Message  string   `json:"message"`
	Errors   []string `json:"errors,omitempty"`
}

func NewTicketHandler(httpClient HttpClient, database db.Database) *ticketHandler {
	return &ticketHandler{
		httpClient: httpClient,
		db:         database,
	}
}

func (th *ticketHandler) GetTicket(w http.ResponseWriter, r *http.Request) {
	uuid := chi.URLParam(r, "uuid")
	if uuid == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "UUID is required"})
		return
	}

	ticket, err := th.db.GetTicket(uuid)
	if err != nil {
		if err.Error() == "ticket not found" {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "Ticket not found"})
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": fmt.Sprintf("Failed to get ticket: %v", err)})
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(ticket)
}

func (th *ticketHandler) UpdateTicket(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	pubKeyFromAuth, _ := ctx.Value(auth.ContextKey).(string)

	if pubKeyFromAuth == "" {
		fmt.Println("[ticket] no pubkey from auth")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "Unauthorized"})
		return
	}

	uuidStr := chi.URLParam(r, "uuid")
	if uuidStr == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "UUID is required"})
		return
	}

	ticketUUID, err := uuid.Parse(uuidStr)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Invalid UUID format"})
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Error reading request body"})
		return
	}
	defer r.Body.Close()

	var ticket db.Tickets
	if err := json.Unmarshal(body, &ticket); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Error parsing request body"})
		return
	}

	ticket.UUID = ticketUUID

	if ticket.Status != "" && !db.IsValidTicketStatus(ticket.Status) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Invalid ticket status"})
		return
	}

	updatedTicket, err := th.db.CreateOrEditTicket(&ticket)
	if err != nil {
		if err.Error() == "feature_uuid, phase_uuid, and name are required" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": fmt.Sprintf("Failed to update ticket: %v", err)})
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(updatedTicket)
}

func (th *ticketHandler) DeleteTicket(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	pubKeyFromAuth, _ := ctx.Value(auth.ContextKey).(string)

	if pubKeyFromAuth == "" {
		fmt.Println("[ticket] no pubkey from auth")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "Unauthorized"})
		return
	}

	uuid := chi.URLParam(r, "uuid")
	if uuid == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "UUID is required"})
		return
	}

	err := th.db.DeleteTicket(uuid)
	if err != nil {
		if err.Error() == "ticket not found" {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "Ticket not found"})
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": fmt.Sprintf("Failed to delete ticket: %v", err)})
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"message": "Ticket deleted successfully"})
}

func (th *ticketHandler) PostTicketDataToStakwork(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	pubKeyFromAuth, _ := ctx.Value(auth.ContextKey).(string)

	if pubKeyFromAuth == "" {
		fmt.Println("[ticket] no pubkey from auth")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "Unauthorized"})
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(TicketResponse{
			Success: false,
			Message: "Validation failed",
			Errors:  []string{"Error reading request body"},
		})
		return
	}
	defer r.Body.Close()

	var ticket db.Tickets
	if err := json.Unmarshal(body, &ticket); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(TicketResponse{
			Success: false,
			Message: "Validation failed",
			Errors:  []string{"Error parsing request body: " + err.Error()},
		})
		return
	}

	var validationErrors []string
	if ticket.UUID == uuid.Nil {
		validationErrors = append(validationErrors, "UUID is required")
	} else {
		if _, err := uuid.Parse(ticket.UUID.String()); err != nil {
			validationErrors = append(validationErrors, "Invalid UUID format")
		}
	}

	if len(validationErrors) > 0 {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(TicketResponse{
			Success: false,
			Message: "Validation failed",
			Errors:  validationErrors,
		})
		return
	}

	var productBrief, featureBrief string
	if ticket.FeatureUUID != "" {
		feature := th.db.GetFeatureByUuid(ticket.FeatureUUID)
		if feature.Uuid == "" {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(TicketResponse{
				Success: false,
				Message: "Error retrieving feature details",
				Errors:  []string{"Feature not found with the provided UUID"},
			})
			return
		}

		var err error
		productBrief, err = th.db.GetProductBrief(feature.WorkspaceUuid)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(TicketResponse{
				Success: false,
				Message: "Error retrieving product brief",
				Errors:  []string{err.Error()},
			})
			return
		}

		featureBrief, err = th.db.GetFeatureBrief(ticket.FeatureUUID)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(TicketResponse{
				Success: false,
				Message: "Error retrieving feature brief",
				Errors:  []string{err.Error()},
			})
			return
		}
	}

	host := os.Getenv("HOST")
	if host == "" {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(TicketResponse{
			Success: false,
			Message: "HOST environment variable not set",
		})
		return
	}

	webhookURL := fmt.Sprintf("%s/bounties/ticket/review/", host)

	stakworkPayload := map[string]interface{}{
		"name":        "Hive Ticket Builder",
		"workflow_id": 37324,
		"workflow_params": map[string]interface{}{
			"set_var": map[string]interface{}{
				"attributes": map[string]interface{}{
					"vars": map[string]interface{}{
						"featureUUID":       ticket.FeatureUUID,
						"phaseUUID":         ticket.PhaseUUID,
						"ticketUUID":        ticket.UUID.String(),
						"ticketName":        ticket.Name,
						"ticketDescription": ticket.Description,
						"productBrief":      productBrief,
						"featureBrief":      featureBrief,
						"examples":          "",
						"webhook_url":       webhookURL,
					},
				},
			},
		},
	}

	stakworkPayloadJSON, err := json.Marshal(stakworkPayload)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(TicketResponse{
			Success: false,
			Message: "Error encoding payload",
			Errors:  []string{err.Error()},
		})
		return
	}

	apiKey := os.Getenv("SWWFKEY")
	if apiKey == "" {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(TicketResponse{
			Success: false,
			Message: "API key not set in environment",
		})
		return
	}

	req, err := http.NewRequest(http.MethodPost, "https://api.stakwork.com/api/v1/projects", bytes.NewBuffer(stakworkPayloadJSON))
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(TicketResponse{
			Success: false,
			Message: "Error creating request",
			Errors:  []string{err.Error()},
		})
		return
	}

	req.Header.Set("Authorization", "Token token="+apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := th.httpClient.Do(req)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(TicketResponse{
			Success: false,
			Message: "Error sending request to Stakwork",
			Errors:  []string{err.Error()},
		})
		return
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(TicketResponse{
			Success: false,
			Message: "Error reading response from Stakwork",
			Errors:  []string{err.Error()},
		})
		return
	}

	if resp.StatusCode != http.StatusOK {
		w.WriteHeader(resp.StatusCode)
		json.NewEncoder(w).Encode(TicketResponse{
			Success: false,
			Message: string(respBody),
			Errors:  []string{fmt.Sprintf("Stakwork API returned status code: %d", resp.StatusCode)},
		})
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(TicketResponse{
		Success:  true,
		Message:  string(respBody),
		TicketID: ticket.UUID.String(),
	})
}

func (th *ticketHandler) ProcessTicketReview(w http.ResponseWriter, r *http.Request) {

	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("Error reading request body: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Error reading request body"})
		return
	}
	defer r.Body.Close()

	var reviewReq utils.TicketReviewRequest
	if err := json.Unmarshal(body, &reviewReq); err != nil {
		log.Printf("Error parsing request JSON: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Error parsing request body"})
		return
	}

	if err := utils.ValidateTicketReviewRequest(&reviewReq); err != nil {
		log.Printf("Invalid request data: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	ticket, err := th.db.GetTicket(reviewReq.TicketUUID)
	if err != nil {
		log.Printf("Error fetching ticket: %v", err)
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "Ticket not found"})
		return
	}

	ticket.Description = reviewReq.TicketDescription

	updatedTicket, err := th.db.UpdateTicket(ticket)
	if err != nil {
		log.Printf("Error updating ticket: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Failed to update ticket"})
		return
	}

	log.Printf("Successfully updated ticket %s", reviewReq.TicketUUID)
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(updatedTicket)
}

func (th *ticketHandler) GetTicketsByPhaseUUID(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	pubKeyFromAuth, _ := ctx.Value(auth.ContextKey).(string)
	if pubKeyFromAuth == "" {
		fmt.Println("no pubkey from auth")
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	featureUUID := chi.URLParam(r, "feature_uuid")
	phaseUUID := chi.URLParam(r, "phase_uuid")

	if _, err := uuid.Parse(featureUUID); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid feature UUID format"})
		return
	}
	if _, err := uuid.Parse(phaseUUID); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid phase UUID format"})
		return
	}

	feature := th.db.GetFeatureByUuid(featureUUID)
	if feature.Uuid == "" {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "feature not found"})
		return
	}

	_, err := th.db.GetFeaturePhaseByUuid(featureUUID, phaseUUID)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "Phase not found"})
		return
	}

	tickets, err := th.db.GetTicketsByPhaseUUID(featureUUID, phaseUUID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(tickets)
}