package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"google.golang.org/appengine"
	"google.golang.org/appengine/log"
)

type DialogueFlowRequest struct {
	ResponseID                  string                      `json:"responseId"`
	QueryResult                 DialogueFlowQueryResult     `json:"queryResult"`
	OriginalDetectIntentRequest OriginalDetectIntentRequest `json:"originalDetectIntentRequest"`
	Session                     string                      `json:"session"`
}

type DialogueFlowQueryResult struct {
	QueryText                string                 `json:"queryText"`
	Action                   string                 `json:"action"`
	Parameters               map[string]interface{} `json:"parameters"`
	AllRequiredParamsPresent bool                   `json:"allRequiredParamsPresent"`
	Intent                   struct {
		Name        string `json:"name"`
		DisplayName string `json:"displayName"`
	} `json:"intent"`
	IntentDetectionConfidence float64 `json:"intentDetectionConfidence"`
	DiagnosticInfo            struct {
	} `json:"diagnosticInfo"`
	LanguageCode string `json:"languageCode"`
}
type OriginalDetectIntentRequest struct {
	Payload struct {
		Data struct {
			AuthedUsers []string `json:"authed_users"`
			EventID     string   `json:"event_id"`
			APIAppID    string   `json:"api_app_id"`
			TeamID      string   `json:"team_id"`
			Event       struct {
				EventTs string `json:"event_ts"`
				Channel string `json:"channel"`
				Text    string `json:"text"`
				Type    string `json:"type"`
				User    string `json:"user"`
				Ts      string `json:"ts"`
			} `json:"event"`
			Type      string  `json:"type"`
			EventTime float64 `json:"event_time"`
			Token     string  `json:"token"`
		} `json:"data"`
		Source string `json:"source"`
	} `json:"payload"`
}
type DialogueFlowParameter struct {
	name  string
	value string
}
type DialogueFlowResponse struct {
	FulfillmentText     string `json:"fulfillmentText"`
	FulfillmentMessages []struct {
		Card struct {
			Title    string `json:"title"`
			Subtitle string `json:"subtitle"`
			ImageURI string `json:"imageUri"`
			Buttons  []struct {
				Text     string `json:"text"`
				Postback string `json:"postback"`
			} `json:"buttons"`
		} `json:"card"`
	} `json:"fulfillmentMessages"`
	Source  string `json:"source"`
	Payload struct {
		Slack SlashRollJSONResponse `json:"slack"`
	} `json:"payload"`
	OutputContexts []struct {
		Name          string `json:"name"`
		LifespanCount int    `json:"lifespanCount"`
		Parameters    struct {
			Param string `json:"param"`
		} `json:"parameters"`
	} `json:"outputContexts"`
	FollowupEventInput struct {
		Name         string `json:"name"`
		LanguageCode string `json:"languageCode"`
		Parameters   struct {
			Param string `json:"param"`
		} `json:"parameters"`
	} `json:"followupEventInput"`
}

func DialogueWebhookHandler(w http.ResponseWriter, r *http.Request) {
	//response := "This is a sample response from your webhook!"
	ctx := appengine.NewContext(r)
	// Save a copy of this request for debugging.
	//requestDump, err := httputil.DumpRequest(r, true)
	//if err != nil {
	//	log.Criticalf(ctx, "%v", err)
	//	return
	//	}
	//	log.Debugf(ctx, "Whole Request: %s", string(requestDump))

	//read body into dialogueFlowRequest
	var dialogueFlowRequest = new(DialogueFlowRequest)
	err := json.NewDecoder(r.Body).Decode(dialogueFlowRequest)
	defer r.Body.Close()
	if err != nil {
		log.Criticalf(ctx, fmt.Sprintf("%+v", err))
	}
	//log a bunch of crap
	log.Debugf(ctx, "Confidence %d\n", dialogueFlowRequest.QueryResult.IntentDetectionConfidence)
	log.Debugf(ctx, "Parameters %+v\n", dialogueFlowRequest.QueryResult.Parameters)
	log.Debugf(ctx, "QueryText: %#v", dialogueFlowRequest.QueryResult.QueryText)
	log.Debugf(ctx, "dialogueFlowRequest.QueryResult.Parameters: %#v",
		dialogueFlowRequest.QueryResult.Parameters)
	log.Debugf(ctx, "dialogueFlowRequest.QueryResult.ParametersDice: %#v",
		dialogueFlowRequest.QueryResult.Parameters["DiceExpression"])

	//switch on Intent
	if strings.Contains(dialogueFlowRequest.QueryResult.Intent.Name, "b41d0bdc-45f0-4099-ac34-40baf8dbb9ec") {
		handleRollIntent(ctx, *dialogueFlowRequest, w, r)
	} else if strings.Contains(dialogueFlowRequest.QueryResult.Intent.Name, "d8cc1857-c36c-4a5e-bef5-8c1b5953c87c") {
		handleDecideIntent(ctx, *dialogueFlowRequest, w, r)
	} else if strings.Contains(dialogueFlowRequest.QueryResult.Intent.Name, "e279adb0-a664-4ef8-874e-9f677208284f") {
		handleCommandIntent(ctx, *dialogueFlowRequest, w, r)
	} else if strings.Contains(dialogueFlowRequest.QueryResult.Intent.Name, "e9609f6a-a4ec-49a4-88a1-5c2265581c2f") {
		handleRememberIntent(ctx, *dialogueFlowRequest, w, r)
	}

}
func handleRememberIntent(ctx context.Context, dialogueFlowRequest DialogueFlowRequest, w http.ResponseWriter, r *http.Request) {
	dialogueFlowResponse := new(DialogueFlowResponse)
	slackRollResponse := SlashRollJSONResponse{}
	diceExpressionCount := len(dialogueFlowRequest.QueryResult.Parameters["DiceExpression"].([]interface{}))
	var command RollCommand
	var diceStrings []string
	for i := 0; i < diceExpressionCount; i++ {
		diceExpressionString := addMissingCloseParens(dialogueFlowRequest.QueryResult.Parameters["DiceExpression"].([]interface{})[i].(string))
		// add ROLL identifier for parser
		if !strings.Contains(strings.ToUpper(diceExpressionString), "ROLL") {
			diceExpressionString = fmt.Sprintf("roll %s", diceExpressionString)
		}
		diceStrings = append(diceStrings, diceExpressionString)
	}
	command.FromString(diceStrings...)
	namespace := dialogueFlowRequest.OriginalDetectIntentRequest.Payload.Data.TeamID
	commandName := "!" + dialogueFlowRequest.QueryResult.Parameters["Command"].(string)
	key := hashStrings(commandName, dialogueFlowRequest.OriginalDetectIntentRequest.Payload.Data.Event.User)
	err := command.Save(ctx, namespace, key)
	log.Debugf(ctx, "command:%s user: %s key: %s", commandName, dialogueFlowRequest.OriginalDetectIntentRequest.Payload.Data.Event.User, key)
	if err != nil {
		printErrorToDialogFlowSlack(ctx, err, w, r)
		return
	}
	var attachment Attachment
	attachment.AuthorName = fmt.Sprintf("Saved %s", commandName)
	slackRollResponse.Attachments = append(slackRollResponse.Attachments, attachment)
	dialogueFlowResponse.Payload.Slack = slackRollResponse
	//Send Response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(dialogueFlowResponse)

}
func handleCommandIntent(ctx context.Context, dialogueFlowRequest DialogueFlowRequest, w http.ResponseWriter, r *http.Request) {
	command := dialogueFlowRequest.QueryResult.QueryText
	var rollCommand RollCommand
	namespace := dialogueFlowRequest.OriginalDetectIntentRequest.Payload.Data.TeamID
	key := hashStrings(command, dialogueFlowRequest.OriginalDetectIntentRequest.Payload.Data.Event.User)
	err := rollCommand.Get(ctx,
		namespace,
		key)

	log.Debugf(ctx, "command:%s user: %s key: %s", command, dialogueFlowRequest.OriginalDetectIntentRequest.Payload.Data.Event.User, key)
	if err != nil {
		printErrorToDialogFlowSlack(ctx, err, w, r)
		return
	}
	handleRollCommand(ctx, rollCommand, w, r)
	key = hashStrings("!!", dialogueFlowRequest.OriginalDetectIntentRequest.Payload.Data.Event.User)
	rollCommand.Save(ctx, namespace, key)

}
func handleRollCommand(ctx context.Context, command RollCommand, w http.ResponseWriter, r *http.Request) {
	dialogueFlowResponse := new(DialogueFlowResponse)
	slackRollResponse := SlashRollJSONResponse{}
	diceExpressionCount := len(command.RollExpresions)
	for i := 0; i < diceExpressionCount; i++ {
		attachment, err := command.RollExpresions[i].ToSlackAttachment()
		if err != nil {
			printErrorToDialogFlowSlack(ctx, err, w, r)
			return
		}
		slackRollResponse.Attachments = append(slackRollResponse.Attachments, attachment)
	}

	dialogueFlowResponse.Payload.Slack = slackRollResponse

	//Send Response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(dialogueFlowResponse)
}

func handleDecideIntent(ctx context.Context, dialogueFlowRequest DialogueFlowRequest, w http.ResponseWriter, r *http.Request) {

	dialogueFlowResponse := new(DialogueFlowResponse)
	slackRollResponse := SlashRollJSONResponse{}

	//create a RollDecision and fill it
	rollDecision := RollDecision{}
	rollDecision.question = dialogueFlowRequest.QueryResult.QueryText

	dflowChoices := dialogueFlowRequest.QueryResult.Parameters["Choices"].([]interface{})

	if len(dflowChoices) < 2 {
		rollDecision.choices = append(rollDecision.choices, "Yes")
		rollDecision.choices = append(rollDecision.choices, "No")
	} else {
		for _, v := range dflowChoices {
			rollDecision.choices = append(rollDecision.choices, strings.Title(v.(string)))
		}
		log.Debugf(ctx, fmt.Sprintf("Choices(%d): %v", len(rollDecision.choices), rollDecision.choices))
	}
	result, _ := roll(int64(1), int64(len(rollDecision.choices)))
	rollDecision.result = result - 1

	log.Debugf(ctx, fmt.Sprintf("RollDecision:\n%+v", rollDecision))

	//create a slack attachment from RollDecision
	attachment, _ := rollDecision.ToSlackAttachment()
	//attach it to Slack payload
	slackRollResponse.Attachments = append(slackRollResponse.Attachments, attachment)
	slackRollResponse.Text = "I'll roll some dice to help you make that decision."
	dialogueFlowResponse.Payload.Slack = slackRollResponse
	//log.Debugf(ctx, spew.Sprintf("My Response:\n%+v", dialogueFlowResponse))
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(dialogueFlowResponse)
}
func handleRollIntent(ctx context.Context, dialogueFlowRequest DialogueFlowRequest, w http.ResponseWriter, r *http.Request) {
	diceExpressionCount := len(dialogueFlowRequest.QueryResult.Parameters["DiceExpression"].([]interface{}))
	var command RollCommand
	var diceStrings []string
	for i := 0; i < diceExpressionCount; i++ {
		diceExpressionString := addMissingCloseParens(dialogueFlowRequest.QueryResult.Parameters["DiceExpression"].([]interface{})[i].(string))
		// add ROLL identifier for parser
		if !strings.Contains(strings.ToUpper(diceExpressionString), "ROLL") {
			diceExpressionString = fmt.Sprintf("roll %s", diceExpressionString)
		}
		diceStrings = append(diceStrings, diceExpressionString)
	}
	command.FromString(diceStrings...)

	//Save for replay
	namespace := dialogueFlowRequest.OriginalDetectIntentRequest.Payload.Data.TeamID
	key := hashStrings("!!", dialogueFlowRequest.OriginalDetectIntentRequest.Payload.Data.Event.User)
	command.Save(ctx, namespace, key)
	//
	handleRollCommand(ctx, command, w, r)
}

func printErrorToDialogFlowSlack(ctx context.Context, err error, w http.ResponseWriter, r *http.Request) {
	dialogueFlowResponse := new(DialogueFlowResponse)
	slackRollResponse := SlashRollJSONResponse{}
	slackRollResponse.Text = err.Error()
	dialogueFlowResponse.Payload.Slack = slackRollResponse
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(dialogueFlowResponse)
}

func addMissingCloseParens(text string) string {
	if strings.Count(text, ")") < strings.Count(text, "(") {
		text += ")"
		return addMissingCloseParens(text)
	}
	if strings.Count(text, "]") < strings.Count(text, "[") {
		text += "]"
		return addMissingCloseParens(text)
	}
	return text
}