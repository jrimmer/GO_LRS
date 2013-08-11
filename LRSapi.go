package main

import (
	"encoding/json"
	"fmt"
	"github.com/gorilla/mux"
	"github.com/nu7hatch/gouuid"
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/bson"
	"net/http"
)

func main() {
	r := mux.NewRouter()

	r.Path("/statements/").Methods("POST").HandlerFunc(PostStatement)
	r.Path("/statements/").Methods("PUT").HandlerFunc(PutStatement)
	r.Path("/statements/").Methods("GET").HandlerFunc(GetStatement)

	http.Handle("/", r)
	http.ListenAndServe(":8080", nil)
}

//todo make index
func dbSession() *mgo.Session {
	session, err := mgo.Dial("localhost")
	if err != nil {
		panic(err)
	}
	return session
}

func PostStatement(w http.ResponseWriter, r *http.Request) {

	statementId := r.FormValue("statementId")
	if statementId != "" {
		PutStatement(w, r)
		return
	}

	decoder := json.NewDecoder(r.Body)
	defer r.Body.Close()

	var statements []Statement
	err := decoder.Decode(&statements)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		// fmt.Fprint(w, err)
		return
	}

	// connect to db
	session := dbSession()
	defer session.Close()
	statementsC := session.DB("LRS").C("statements")

	// check if trying to replace object with same id
	status := checkIdConflictBatch(w, statementsC, statements)
	if status != 0 {
		w.WriteHeader(status)
		// fmt.Fprint(w, status)
		return
	}

	var sids []string
	for _, s := range statements {

		sid, err := s.Validate()
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			// fmt.Fprint(w, err)
			return
		}
		// output new ids
		sids = append(sids, sid)

		// save to db
		statementsC.Insert(s)
	}

	// return 200 with statement id(s), same order index
	w.Header().Add("Content-Type", "application/json")
	w.Header().Add("X-Experience-API-Version", "1.0")
	w.WriteHeader(http.StatusOK)

	fmt.Fprint(w, sids)
	return
}

func PutStatement(w http.ResponseWriter, r *http.Request) {

	// verify statementId passed in
	statementId := r.FormValue("statementId")
	if statementId == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	decoder := json.NewDecoder(r.Body)
	defer r.Body.Close()

	var s Statement
	err := decoder.Decode(&s)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		// fmt.Fprint(w, err)
		return
	}

	// connect to db
	session := dbSession()
	defer session.Close()
	statementsC := session.DB("LRS").C("statements")

	// check if trying to replace object with same id
	s.Id = statementId
	status := checkIdConflict(w, statementsC, s)
	if status != 0 {
		w.WriteHeader(status)
		// fmt.Fprint(w, status)
		return
	}

	_, err = s.Validate()
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		// fmt.Fprint(w, err)
		return
	}
	// save to db
	statementsC.Insert(s)
}

func checkIdConflictBatch(w http.ResponseWriter, statementsC *mgo.Collection, statements []Statement) int {

	// build array of IDs to query if statement(s) exist
	var IDs []string
	Lkup := make(map[string]Statement)

	for _, s := range statements {
		if s.Id != "" {
			IDs = append(IDs, s.Id)
			Lkup[s.Id] = s
		}
	}

	// if so then check if they are the same object so as not to throw conflict
	if IDs != nil {
		var result []Statement
		err := statementsC.Find(bson.M{"id": bson.M{"$in": IDs}}).All(&result)
		if err != nil {
			// fmt.Fprint(w, err)
			return http.StatusBadRequest
		}

		for _, s := range result {
			// can't compare structs with arrays/maps
			rj, _ := json.Marshal(Lkup[s.Id])
			sj, _ := json.Marshal(s)
			if string(rj) != string(sj) {
				// fmt.Fprint(w, "error conflict")
				return http.StatusConflict
			}
		}
	}
	return 0
}

func checkIdConflict(w http.ResponseWriter, statementsC *mgo.Collection, statement Statement) int {

	if statement.Id != "" {
		var result Statement
		err := statementsC.Find(bson.M{"id": statement.Id}).One(&result)

		// can't compare structs with arrays/maps
		rj, _ := json.Marshal(result)
		sj, _ := json.Marshal(statement)
		if err == nil && string(rj) != string(sj) {
			// fmt.Fprint(w, "error conflict")
			return http.StatusConflict
		}
	}
	return 0
}

// https://github.com/adlnet/ADL_LRS/blob/d86aa83ec5674982a233bae5a90df5288c8209d0/lrs/util/retrieve_statement.py
func GetStatement(w http.ResponseWriter, r *http.Request) {

   	// --validate
   	// check if format ['exact', 'canonical', 'ids'] default exact
   	// check if	contain both statementId and voidedStatementId parameters then 400
   	// check contain any other parameter besides "attachments" or "format".
   	/* not_allowed = ["agent", "verb", "activity", "registration",
                       "related_activities", "related_agents", "since",
                       "until", "limit", "ascending"]*/

	// --query db
	// check id exists or voided
	// The LRS MUST not return any Statement which has been voided, unless that Statement has been requested by voidedStatementId.
	// The LRS MUST still return any Statements targeting the voided Statement when retrieving Statements using explicit
	// or implicit time or sequence based retrieval, unless they themselves have been voided.
	// query complex get
	// https://github.com/adlnet/xAPI-Spec/blob/master/xAPI.md#stmtapi
	// based on "stored" time, subject to permissions and maximum list length.
	// create cache for more statements due to limit
	// format StatementResult {statements [], more IRL (link to more via querystring if "limit" set)} with list in newest stored first if "ascending" not set
	// return 200 statementResults with proper header
}

// https://github.com/adlnet/xAPI-Spec/blob/master/xAPI.md#stmtapi
// http://zackpierce.github.io/xAPI-Validator-JS/
// not sure how much if/howmuch I will validate structure
func (s *Statement) Validate() (string, error) {
	// generate new ID's
	if s.Id == "" {
		id, _ := uuid.NewV4()
		s.Id = id.String()
	}
	return s.Id, nil
}

// -----------------------------------------------------------------
// import github.com/bitly/go-simplejson maybe instead
type Statement struct {
	Id          string
	Actor       Actor
	Verb        Verb
	Object      Object
	Result      Result
	Context     Context
	Timestamp   string
	Stored      string
	Authority   Actor
	Version     string
	Attachments []Attachment
}

// statement
type Actor struct {
	ObjectType   string  `json:",omitempty"`
	Name         string  `json:",omitempty"`
	Mbox         string  `json:",omitempty"`
	Mbox_sha1sum string  `json:",omitempty"`
	OpenID       string  `json:",omitempty"`
	Account      Account `json:",omitempty"`
	// group
	Member []Actor `json:",omitempty"`
}

// actor
type Agent struct {
	ObjectType   string  `json:",omitempty"`
	Name         string  `json:",omitempty"`
	Mbox         string  `json:",omitempty"`
	Mbox_sha1sum string  `json:",omitempty"`
	OpenID       string  `json:",omitempty"`
	Account      Account `json:",omitempty"`
}

// actor
type Account struct {
	HomePage string
	Name     string
}

// statement
type Verb struct {
	Id      string
	Display map[string]string
}

// activity, Agent/Group, Sub-Statement, StatementReference
type Object struct {
	ObjectType string     `json:",omitempty"`
	Id         string     `json:",omitempty"`
	Definition Definition `json:",omitempty"`
	// substatement
	Actor       Actor        `json:",omitempty"`
	Verb        Verb         `json:",omitempty"`
	Object      StatementRef `json:",omitempty"`
	Result      Result       `json:",omitempty"`
	Context     Context      `json:",omitempty"`
	Timestamp   string       `json:",omitempty"`
	Stored      string       `json:",omitempty"`
	Authority   Actor        `json:",omitempty"`
	Version     string       `json:",omitempty"`
	Attachments []Attachment `json:",omitempty"`
}

// object
type Definition struct {
	Name        map[string]string
	Description map[string]string
	Type        string
	MoreInfo    string
	Interaction Interaction
	Extensions  map[string]interface{}
}

// definition
type Interaction struct {
	InteractionType         string
	CorrectResponsesPattern []string
	choices                 []InteractionComponents `json:",omitempty"`
	scale                   []InteractionComponents `json:",omitempty"`
	source                  []InteractionComponents `json:",omitempty"`
	target                  []InteractionComponents `json:",omitempty"`
	steps                   []InteractionComponents `json:",omitempty"`
}

// interaction
type InteractionComponents struct {
	Id          string
	Description map[string]string
}

// statement
type Result struct {
	Score      Score
	Success    bool
	Completion bool
	Response   string
	Duration   string
	Extensions map[string]interface{}
}

// result
type Score struct {
	Scaled int
	Raw    float32
	Min    float32
	Max    float32
}

// statement
type Context struct {
	Registration      string
	Instructor        Actor
	Team              Actor
	ContextActivities map[string]interface{}
	Revision          string
	Platform          string
	Language          string
	Statement         StatementRef
	Extensions        map[string]interface{}
}

// context
type StatementRef struct {
	ObjectType string
	Id         string
	Definition Definition `json:",omitempty"`
}

// statement
type Attachment struct {
	UsageType   string
	Display     map[string]string
	Description map[string]string `json:",omitempty"`
	ContentType string
	Length      int
	Sha2        string
	FileUrl     string `json:",omitempty"`
}