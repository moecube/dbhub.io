package main

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	gfm "github.com/justinclift/github_flavored_markdown"
	com "github.com/sqlitebrowser/dbhub.io/common"
)

// Renders the "About Us" page.
func aboutPage(w http.ResponseWriter, r *http.Request) {
	var pageData struct {
		Auth0 com.Auth0Set
		Meta  com.MetaInfo
	}

	// Retrieve session data (if any)
	var loggedInUser string
	sess, err := store.Get(r, "dbhub-user")
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}
	u := sess.Values["UserName"]
	if u != nil {
		loggedInUser = u.(string)
		pageData.Meta.LoggedInUser = loggedInUser
	}

	pageData.Meta.Title = "What is DBHub.io?"

	// Add Auth0 info to the page data
	pageData.Auth0.CallbackURL = "https://" + com.Conf.Web.ServerName + "/x/callback"
	pageData.Auth0.ClientID = com.Conf.Auth0.ClientID
	pageData.Auth0.Domain = com.Conf.Auth0.Domain

	// Render the page
	t := tmpl.Lookup("aboutPage")
	err = t.Execute(w, pageData)
	if err != nil {
		log.Printf("Error: %s", err)
	}
}

// Render the branches page, which lists the branches for a database.
func branchesPage(w http.ResponseWriter, r *http.Request) {
	// Structure to hold page data
	type brEntry struct {
		Commit       string `json:"commit"`
		Description  string `json:"description"`
		MarkDownDesc string `json:"mkdowndesc"`
		Name         string `json:"name"`
	}
	var pageData struct {
		Auth0         com.Auth0Set
		Branches      []brEntry
		DB            com.SQLiteDBinfo
		DefaultBranch string
		Meta          com.MetaInfo
	}
	pageData.Meta.Title = "Branch list"

	// Retrieve session data (if any)
	var loggedInUser string
	sess, err := store.Get(r, "dbhub-user")
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}
	u := sess.Values["UserName"]
	if u != nil {
		loggedInUser = u.(string)
		pageData.Meta.LoggedInUser = loggedInUser
	}

	// Retrieve the database owner & name
	// TODO: Add folder and branch name support
	dbFolder := "/"
	dbOwner, dbName, err := com.GetOD(1, r) // 1 = Ignore "/branches/" at the start of the URL
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Validate the supplied information
	if dbOwner == "" || dbName == "" {
		errorPage(w, r, http.StatusBadRequest, "Missing database owner or database name")
		return
	}

	// Check if the requested database exists
	exists, err := com.CheckDBExists(loggedInUser, dbOwner, dbFolder, dbName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	if !exists {
		errorPage(w, r, http.StatusNotFound, fmt.Sprintf("Database '%s%s%s' doesn't exist", dbOwner, dbFolder,
			dbName))
		return
	}

	// Check if the user has access to the requested database (and get it's details if available)
	err = com.DBDetails(&pageData.DB, loggedInUser, dbOwner, dbFolder, dbName, "")
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Read the branch heads list from the database
	branches, err := com.GetBranches(dbOwner, dbFolder, dbName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	pageData.DefaultBranch, err = com.GetDefaultBranchName(dbOwner, dbFolder, dbName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// Fill out the metadata
	pageData.Meta.Owner = dbOwner
	pageData.Meta.Database = dbName

	for i, j := range branches {
		// Create a branch entry
		k := brEntry{
			Commit:       j.Commit,
			Description:  j.Description,
			MarkDownDesc: string(gfm.Markdown([]byte(j.Description))),
			Name:         i,
		}
		pageData.Branches = append(pageData.Branches, k)
	}

	// Add Auth0 info to the page data
	pageData.Auth0.CallbackURL = "https://" + com.Conf.Web.ServerName + "/x/callback"
	pageData.Auth0.ClientID = com.Conf.Auth0.ClientID
	pageData.Auth0.Domain = com.Conf.Auth0.Domain

	// Render the page
	t := tmpl.Lookup("branchesPage")
	err = t.Execute(w, pageData)
	if err != nil {
		log.Printf("Error: %s", err)
	}
}

// Render the commits page.  This shows all of the commits in a given branch, in reverse order from newest to oldest.
func commitsPage(w http.ResponseWriter, r *http.Request) {
	// Structure to hold page data
	type HistEntry struct {
		AuthorEmail    string     `json:"author_email"`
		AuthorName     string     `json:"author_name"`
		AuthorUserName string     `json:"author_user_name"`
		CommitterEmail string     `json:"committer_email"`
		CommitterName  string     `json:"committer_name"`
		ID             string     `json:"id"`
		Message        string     `json:"message"`
		Parent         string     `json:"parent"`
		Timestamp      time.Time  `json:"timestamp"`
		Tree           com.DBTree `json:"tree"`
	}
	var pageData struct {
		Auth0    com.Auth0Set
		Branch   string
		Branches []string
		DB       com.SQLiteDBinfo
		History  []HistEntry
		Meta     com.MetaInfo
	}
	pageData.Meta.Title = "Commits settings"

	// Retrieve session data (if any)
	var loggedInUser string
	sess, err := store.Get(r, "dbhub-user")
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}
	u := sess.Values["UserName"]
	if u != nil {
		loggedInUser = u.(string)
		pageData.Meta.LoggedInUser = loggedInUser
	}

	// Retrieve the database owner & name, and branch name
	// TODO: Add folder support
	dbFolder := "/"
	dbOwner, dbName, err := com.GetOD(1, r) // 1 = Ignore "/settings/" at the start of the URL
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}
	branchName, err := com.GetFormBranch(r)
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Validate the supplied information
	if dbOwner == "" || dbName == "" {
		errorPage(w, r, http.StatusBadRequest, "Missing database owner or database name")
		return
	}

	// Check if the requested database exists
	exists, err := com.CheckDBExists(loggedInUser, dbOwner, dbFolder, dbName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	if !exists {
		errorPage(w, r, http.StatusNotFound, fmt.Sprintf("Database '%s%s%s' doesn't exist", dbOwner, dbFolder,
			dbName))
		return
	}

	// Check if the user has access to the requested database (and get it's details if available)
	err = com.DBDetails(&pageData.DB, loggedInUser, dbOwner, dbFolder, dbName, "")
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Read the branch heads list from the database
	branches, err := com.GetBranches(dbOwner, dbFolder, dbName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// If no branch name was given, we use the default branch
	if branchName == "" {
		branchName, err = com.GetDefaultBranchName(dbOwner, dbFolder, dbName)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, err.Error())
			return
		}
	}

	// Work out the head commit ID for the requested branch
	headCom, ok := branches[branchName]
	headID := headCom.Commit
	if !ok {
		// Unknown branch
		errorPage(w, r, http.StatusInternalServerError, fmt.Sprintf("Branch '%s' not found", branchName))
		return
	}
	if headID == "" {
		// The requested branch wasn't found.  Bad request?
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Walk the commit history backwards from the head commit, assembling the commit history for this branch from the
	// full list
	rawList, err := com.GetCommitList(dbOwner, dbFolder, dbName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	// TODO: Ugh, this is an ugly approach just to add the username to the commit data.  Surely there's a better way?
	// TODO  Maybe store the username in the commit data structure in the database instead?
	// TODO: Display licence changes too
	eml, err := com.GetUsernameFromEmail(rawList[headID].AuthorEmail)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// Create the history entry
	pageData.History = []HistEntry{
		{
			AuthorEmail:    rawList[headID].AuthorEmail,
			AuthorName:     rawList[headID].AuthorName,
			AuthorUserName: eml,
			CommitterEmail: rawList[headID].CommitterEmail,
			CommitterName:  rawList[headID].CommitterName,
			ID:             rawList[headID].ID,
			Message:        string(gfm.Markdown([]byte(rawList[headID].Message))),
			Parent:         rawList[headID].Parent,
			Timestamp:      rawList[headID].Timestamp,
			Tree:           rawList[headID].Tree,
		},
	}
	commitData := com.CommitEntry{Parent: rawList[headID].Parent}
	for commitData.Parent != "" {
		commitData, ok = rawList[commitData.Parent]
		if !ok {
			errorPage(w, r, http.StatusInternalServerError, "Internal error when retrieving commit data")
			return
		}
		eml, err := com.GetUsernameFromEmail(commitData.AuthorEmail)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, err.Error())
			return
		}

		// Create a history entry
		newEntry := HistEntry{
			AuthorEmail:    commitData.AuthorEmail,
			AuthorName:     commitData.AuthorName,
			AuthorUserName: eml,
			CommitterEmail: commitData.CommitterEmail,
			CommitterName:  commitData.CommitterName,
			ID:             commitData.ID,
			Message:        string(gfm.Markdown([]byte(commitData.Message))),
			Parent:         commitData.Parent,
			Timestamp:      commitData.Timestamp,
			Tree:           commitData.Tree,
		}
		pageData.History = append(pageData.History, newEntry)
	}

	// Fill out the metadata
	pageData.Meta.Owner = dbOwner
	pageData.Meta.Database = dbName
	pageData.Branch = branchName
	for i := range branches {
		pageData.Branches = append(pageData.Branches, i)
	}

	// Add Auth0 info to the page data
	pageData.Auth0.CallbackURL = "https://" + com.Conf.Web.ServerName + "/x/callback"
	pageData.Auth0.ClientID = com.Conf.Auth0.ClientID
	pageData.Auth0.Domain = com.Conf.Auth0.Domain

	// Render the page
	t := tmpl.Lookup("commitsPage")
	err = t.Execute(w, pageData)
	if err != nil {
		log.Printf("Error: %s", err)
	}
}

// Displays a web page asking the user to confirm deleting their database.
func confirmDeletePage(w http.ResponseWriter, r *http.Request) {
	var pageData struct {
		Auth0 com.Auth0Set
		Meta  com.MetaInfo
	}
	pageData.Meta.Title = "Confirm database deletion"

	// Retrieve session data (if any)
	var loggedInUser string
	validSession := false
	sess, err := store.Get(r, "dbhub-user")
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}
	u := sess.Values["UserName"]
	if u != nil {
		loggedInUser = u.(string)
		pageData.Meta.LoggedInUser = loggedInUser
		validSession = true
	}

	// Ensure we have a valid logged in user
	if validSession != true {
		errorPage(w, r, http.StatusUnauthorized, "You need to be logged in")
		return
	}

	// Retrieve the owner and database name
	dbOwner, dbName, err := com.GetOD(1, r) // "1" means skip the first URL word
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, "Validation failed for owner or database value")
		return
	}
	// TODO: Add folder support
	dbFolder := "/"

	// Check if the requested database exists
	exists, err := com.CheckDBExists(loggedInUser, dbOwner, dbFolder, dbName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	if !exists {
		errorPage(w, r, http.StatusNotFound, fmt.Sprintf("Database '%s%s%s' doesn't exist", dbOwner, dbFolder,
			dbName))
		return
	}

	// Make sure the database owner matches the logged in user
	if loggedInUser != dbOwner {
		errorPage(w, r, http.StatusUnauthorized, "You can't change databases you don't own")
		return
	}

	// Fill out metadata for the page to be rendered
	pageData.Meta.Owner = dbOwner
	pageData.Meta.Database = dbName

	// Add Auth0 info to the page data
	pageData.Auth0.CallbackURL = "https://" + com.Conf.Web.ServerName + "/x/callback"
	pageData.Auth0.ClientID = com.Conf.Auth0.ClientID
	pageData.Auth0.Domain = com.Conf.Auth0.Domain

	// Render the page
	t := tmpl.Lookup("confirmDeletePage")
	err = t.Execute(w, pageData)
	if err != nil {
		log.Printf("Error: %s", err)
	}
}

// Render the contributors page, which lists the contributors to a database.
func contributorsPage(w http.ResponseWriter, r *http.Request) {
	// Structures to hold page data
	type AuthorEntry struct {
		AuthorEmail    string `json:"author_email"`
		AuthorName     string `json:"author_name"`
		AuthorUserName string `json:"author_user_name"`
		NumCommits     int    `json:"num_commits"`
	}
	var pageData struct {
		Auth0        com.Auth0Set
		Contributors map[string]AuthorEntry
		DB           com.SQLiteDBinfo
		Meta         com.MetaInfo
	}
	pageData.Meta.Title = "Branch list"

	// Retrieve session data (if any)
	var loggedInUser string
	sess, err := store.Get(r, "dbhub-user")
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}
	u := sess.Values["UserName"]
	if u != nil {
		loggedInUser = u.(string)
		pageData.Meta.LoggedInUser = loggedInUser
	}

	// Retrieve the database owner & name
	// TODO: Add folder and branch support
	dbFolder := "/"
	dbOwner, dbName, err := com.GetOD(1, r) // 1 = Ignore "/branches/" at the start of the URL
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Validate the supplied information
	if dbOwner == "" || dbName == "" {
		errorPage(w, r, http.StatusBadRequest, "Missing database owner or database name")
		return
	}

	// Check if the requested database exists
	exists, err := com.CheckDBExists(loggedInUser, dbOwner, dbFolder, dbName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	if !exists {
		errorPage(w, r, http.StatusNotFound, fmt.Sprintf("Database '%s%s%s' doesn't exist", dbOwner, dbFolder,
			dbName))
		return
	}

	// Check if the user has access to the requested database (and get it's details if available)
	err = com.DBDetails(&pageData.DB, loggedInUser, dbOwner, dbFolder, dbName, "")
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Read the commit list from the database
	commitList, err := com.GetCommitList(dbOwner, dbFolder, dbName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// Fill out the metadata
	pageData.Meta.Owner = dbOwner
	pageData.Meta.Database = dbName
	pageData.Contributors = make(map[string]AuthorEntry)
	for _, j := range commitList {
		// Look up the author's username
		// TODO: There are likely a bunch of ways to optimise this, from keeping the user name entries in a map to
		// TODO  directly storing the username in the jsonb commit data.  Storing the user name entry in the jsonb is
		// TODO  probably the way to go, as it would save lookups in a lot of places
		u, err := com.GetUsernameFromEmail(j.AuthorEmail)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, err.Error())
			return
		}

		// This ok check is just a way to decide whether to increment the NumCommits counter
		_, ok := pageData.Contributors[j.AuthorName]
		if !ok {
			// This is the first time in the loop we're adding the author to the Contributors list
			pageData.Contributors[j.AuthorName] = AuthorEntry{
				AuthorEmail:    j.AuthorEmail,
				AuthorName:     j.AuthorName,
				AuthorUserName: u,
				NumCommits:     1,
			}
		} else {
			// The author is already in the contributors list, so we increment their NumCommits counter
			n := pageData.Contributors[j.AuthorName].NumCommits + 1
			pageData.Contributors[j.AuthorName] = AuthorEntry{
				AuthorEmail:    j.AuthorEmail,
				AuthorName:     j.AuthorName,
				AuthorUserName: u,
				NumCommits:     n,
			}
		}
	}

	// Add Auth0 info to the page data
	pageData.Auth0.CallbackURL = "https://" + com.Conf.Web.ServerName + "/x/callback"
	pageData.Auth0.ClientID = com.Conf.Auth0.ClientID
	pageData.Auth0.Domain = com.Conf.Auth0.Domain

	// Render the page
	t := tmpl.Lookup("contributorsPage")
	err = t.Execute(w, pageData)
	if err != nil {
		log.Printf("Error: %s", err)
	}
}

// Displays a web page asking for the new branch name.
func createBranchPage(w http.ResponseWriter, r *http.Request) {
	var pageData struct {
		Auth0  com.Auth0Set
		Meta   com.MetaInfo
		Commit string
	}
	pageData.Meta.Title = "Create new branch"

	// Retrieve session data (if any)
	var loggedInUser string
	validSession := false
	sess, err := store.Get(r, "dbhub-user")
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}
	u := sess.Values["UserName"]
	if u != nil {
		loggedInUser = u.(string)
		pageData.Meta.LoggedInUser = loggedInUser
		validSession = true
	}

	// Ensure we have a valid logged in user
	if validSession != true {
		errorPage(w, r, http.StatusUnauthorized, "You need to be logged in")
		return
	}

	// Retrieve the owner, database, and commit ID
	dbOwner, dbName, commit, err := com.GetODC(1, r) // "1" means skip the first URL word
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, "Validation failed for commit value")
		return
	}
	// TODO: Add folder support
	dbFolder := "/"

	// Check if the requested database exists
	exists, err := com.CheckDBExists(loggedInUser, dbOwner, dbFolder, dbName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	if !exists {
		errorPage(w, r, http.StatusNotFound, fmt.Sprintf("Database '%s%s%s' doesn't exist", dbOwner, dbFolder,
			dbName))
		return
	}

	// Make sure the database owner matches the logged in user
	if loggedInUser != dbOwner {
		errorPage(w, r, http.StatusUnauthorized, "You can't change databases you don't own")
		return
	}

	// Fill out metadata for the page to be rendered
	pageData.Meta.Owner = dbOwner
	pageData.Meta.Database = dbName
	pageData.Commit = commit

	// Add Auth0 info to the page data
	pageData.Auth0.CallbackURL = "https://" + com.Conf.Web.ServerName + "/x/callback"
	pageData.Auth0.ClientID = com.Conf.Auth0.ClientID
	pageData.Auth0.Domain = com.Conf.Auth0.Domain

	// Render the page
	t := tmpl.Lookup("createBranchPage")
	err = t.Execute(w, pageData)
	if err != nil {
		log.Printf("Error: %s", err)
	}
}

// Displays a web page to input information needed for creating a new discussion.
func createDiscussionPage(w http.ResponseWriter, r *http.Request) {
	var pageData struct {
		Auth0 com.Auth0Set
		Meta  com.MetaInfo
	}
	pageData.Meta.Title = "Create new discussion"

	// Retrieve session data (if any)
	var loggedInUser string
	validSession := false
	sess, err := store.Get(r, "dbhub-user")
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}
	u := sess.Values["UserName"]
	if u != nil {
		loggedInUser = u.(string)
		pageData.Meta.LoggedInUser = loggedInUser
		validSession = true
	}

	// Ensure we have a valid logged in user
	if validSession != true {
		errorPage(w, r, http.StatusUnauthorized, "You need to be logged in")
		return
	}

	// Retrieve the owner, database name
	dbOwner, dbName, err := com.GetOD(1, r) // "1" means skip the first URL word
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}
	// TODO: Add folder support
	dbFolder := "/"

	// Check if the requested database exists
	exists, err := com.CheckDBExists(loggedInUser, dbOwner, dbFolder, dbName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	if !exists {
		errorPage(w, r, http.StatusNotFound, fmt.Sprintf("Database '%s%s%s' doesn't exist", dbOwner, dbFolder,
			dbName))
		return
	}

	// Fill out metadata for the page to be rendered
	pageData.Meta.Owner = dbOwner
	pageData.Meta.Database = dbName

	// Add Auth0 info to the page data
	pageData.Auth0.CallbackURL = "https://" + com.Conf.Web.ServerName + "/x/callback"
	pageData.Auth0.ClientID = com.Conf.Auth0.ClientID
	pageData.Auth0.Domain = com.Conf.Auth0.Domain

	// Render the page
	t := tmpl.Lookup("createDiscussionPage")
	err = t.Execute(w, pageData)
	if err != nil {
		log.Printf("Error: %s", err)
	}
}

// Displays a web page asking for the new tag details.
func createTagPage(w http.ResponseWriter, r *http.Request) {
	var pageData struct {
		Auth0  com.Auth0Set
		Meta   com.MetaInfo
		Commit string
	}
	pageData.Meta.Title = "Create new tag"

	// Retrieve session data (if any)
	var loggedInUser string
	validSession := false
	sess, err := store.Get(r, "dbhub-user")
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}
	u := sess.Values["UserName"]
	if u != nil {
		loggedInUser = u.(string)
		pageData.Meta.LoggedInUser = loggedInUser
		validSession = true
	}

	// Ensure we have a valid logged in user
	if validSession != true {
		errorPage(w, r, http.StatusUnauthorized, "You need to be logged in")
		return
	}

	// Retrieve the owner, database, and commit ID
	dbOwner, dbName, commit, err := com.GetODC(1, r) // "1" means skip the first URL word
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, "Validation failed for commit value")
		return
	}
	// TODO: Add folder support
	dbFolder := "/"

	// Check if the requested database exists
	exists, err := com.CheckDBExists(loggedInUser, dbOwner, dbFolder, dbName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	if !exists {
		errorPage(w, r, http.StatusNotFound, fmt.Sprintf("Database '%s%s%s' doesn't exist", dbOwner, dbFolder,
			dbName))
		return
	}

	// Make sure the database owner matches the logged in user
	if loggedInUser != dbOwner {
		errorPage(w, r, http.StatusUnauthorized, "You can't change databases you don't own")
		return
	}

	// Fill out metadata for the page to be rendered
	pageData.Meta.Owner = dbOwner
	pageData.Meta.Database = dbName
	pageData.Commit = commit

	// Add Auth0 info to the page data
	pageData.Auth0.CallbackURL = "https://" + com.Conf.Web.ServerName + "/x/callback"
	pageData.Auth0.ClientID = com.Conf.Auth0.ClientID
	pageData.Auth0.Domain = com.Conf.Auth0.Domain

	// Render the page
	t := tmpl.Lookup("createTagPage")
	err = t.Execute(w, pageData)
	if err != nil {
		log.Printf("Error: %s", err)
	}
}

func databasePage(w http.ResponseWriter, r *http.Request, dbOwner string, dbFolder string, dbName string) {
	pageName := "Render database page"

	var pageData struct {
		Auth0  com.Auth0Set
		Data   com.SQLiteRecordSet
		DB     com.SQLiteDBinfo
		Meta   com.MetaInfo
		MyStar bool
	}

	// Retrieve session data (if any)
	var loggedInUser string
	sess, err := store.Get(r, "dbhub-user")
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}
	u := sess.Values["UserName"]
	if u != nil {
		loggedInUser = u.(string)
		pageData.Meta.LoggedInUser = loggedInUser
	}

	// Check if a specific database commit ID was given
	commitID, err := com.GetFormCommit(r)
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, "Invalid database commit ID")
		return
	}

	// If a table name was supplied, validate it
	dbTable := r.FormValue("table")
	if dbTable != "" {
		err = com.ValidatePGTable(dbTable)
		if err != nil {
			// Validation failed, so don't pass on the table name
			log.Printf("%s: Validation failed for table name: %s", pageName, err)
			dbTable = ""
		}
	}

	// Check if a branch name was requested
	branchName, err := com.GetFormBranch(r)
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, "Validation failed for branch name")
		return
	}

	// Check if a named tag was requested
	tagName, err := com.GetFormTag(r)
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, "Validation failed for tag name")
		return
	}

	// Check if a specific release was requested
	releaseName := r.FormValue("release")
	if releaseName != "" {
		err = com.Validate.Var(releaseName, "branchortagname,min=1,max=32") // 32 seems a reasonable first guess.
		if err != nil {
			errorPage(w, r, http.StatusBadRequest, "Validation failed for release name")
			return
		}
	}

	// Extract sort column, sort direction, and offset variables if present
	sortCol := r.FormValue("sort")
	sortDir := r.FormValue("dir")
	offsetStr := r.FormValue("offset")

	// If an offset was provided, validate it
	var rowOffset int
	if offsetStr != "" {
		rowOffset, err = strconv.Atoi(offsetStr)
		if err != nil {
			errorPage(w, r, http.StatusBadRequest, err.Error())
			return
		}

		// Ensure the row offset isn't negative
		if rowOffset < 0 {
			rowOffset = 0
		}
	}

	// Sanity check the sort column name
	if sortCol != "" {
		// Validate the sort column text, as we use it in string smashing SQL queries so need to be even more
		// careful than usual
		err = com.ValidateFieldName(sortCol)
		if err != nil {
			log.Printf("Validation failed on requested sort field name '%v': %v\n", sortCol,
				err.Error())
			errorPage(w, r, http.StatusBadRequest, "Validation failed on requested sort field name")
			return
		}
	}

	// If a sort direction was provided, validate it
	if sortDir != "" {
		if sortDir != "ASC" && sortDir != "DESC" {
			errorPage(w, r, http.StatusBadRequest, "Invalid sort direction")
			return
		}
	}

	// Check if the database exists and the user has access to view it
	exists, err := com.CheckDBExists(loggedInUser, dbOwner, dbFolder, dbName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	if !exists {
		errorPage(w, r, http.StatusNotFound, fmt.Sprintf("Database '%s%s%s' doesn't exist", dbOwner, dbFolder,
			dbName))
		return
	}

	// * Execution can only get here if the user has access to the requested database *

	// Increment the view counter for the database (excluding people viewing their own databases)
	if loggedInUser != dbOwner {
		err = com.IncrementViewCount(dbOwner, dbFolder, dbName)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, err.Error())
			return
		}
	}

	// If a specific commit was requested, make sure it exists in the database commit history
	if commitID != "" {
		commitList, err := com.GetCommitList(dbOwner, dbFolder, dbName)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, err.Error())
			return
		}
		_, ok := commitList[commitID]
		if !ok {
			// The requested commit isn't one in the database commit history so error out
			errorPage(w, r, http.StatusNotFound, fmt.Sprintf("Unknown commit for database '%s%s%s'", dbOwner,
				dbFolder, dbName))
			return
		}
	}

	// If a specific release was requested, and no commit ID was given, retrieve the commit ID matching the release
	if commitID == "" && releaseName != "" {
		releases, err := com.GetReleases(dbOwner, dbFolder, dbName)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, "Couldn't retrieve releases for database")
			return
		}
		rls, ok := releases[releaseName]
		if !ok {
			errorPage(w, r, http.StatusInternalServerError, "Unknown release requested for this database")
			return
		}
		commitID = rls.Commit
	}

	// Load the branch info for the database
	branchHeads, err := com.GetBranches(dbOwner, dbFolder, dbName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, "Couldn't retrieve branch information for database")
		return
	}

	// If a specific branch was requested and no commit ID was given, use the latest commit for the branch
	if commitID == "" && branchName != "" {
		c, ok := branchHeads[branchName]
		if !ok {
			errorPage(w, r, http.StatusInternalServerError, "Unknown branch requested for this database")
			return
		}
		commitID = c.Commit
	}

	// If a specific tag was requested, and no commit ID was given, retrieve the commit ID matching the tag
	// TODO: If we need to reduce database calls, we can probably make a function merging this, GetBranches(), and
	// TODO  GetCommitList() above.  Potentially also the DBDetails() call below too.
	if commitID == "" && tagName != "" {
		tags, err := com.GetTags(dbOwner, dbFolder, dbName)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, "Couldn't retrieve tags for database")
			return
		}
		tg, ok := tags[tagName]
		if !ok {
			errorPage(w, r, http.StatusInternalServerError, "Unknown tag requested for this database")
			return
		}
		commitID = tg.Commit
	}

	// Retrieve the database details
	err = com.DBDetails(&pageData.DB, loggedInUser, dbOwner, dbFolder, dbName, commitID)
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// If an sha256 was in the licence field, retrieve it's friendly name and url for displaying
	licSHA := pageData.DB.Info.DBEntry.LicenceSHA
	if licSHA != "" {
		pageData.DB.Info.Licence, pageData.DB.Info.LicenceURL, err = com.GetLicenceInfoFromSha256(dbOwner, licSHA)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, err.Error())
			return
		}
	} else {
		pageData.DB.Info.Licence = "Not specified"
	}

	// Check if the database was starred by the logged in user
	myStar, err := com.CheckDBStarred(loggedInUser, dbOwner, dbFolder, dbName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, "Couldn't retrieve latest social stats")
		return
	}

	// If a specific table wasn't requested, use the user specified default (if present)
	if dbTable == "" {
		// Ensure the default table name validates.  This catches a case where a database was uploaded with an invalid
		// table name and somehow because selected as the default
		a := pageData.DB.Info.DefaultTable
		if a != "" {
			err = com.ValidatePGTable(a)
			if err == nil {
				// The database table name is acceptable, so use it
				dbTable = pageData.DB.Info.DefaultTable
			}
		}
	}

	// Determine the number of rows to display
	var tempMaxRows int
	if loggedInUser != "" {
		tempMaxRows = com.PrefUserMaxRows(loggedInUser)
		pageData.DB.MaxRows = tempMaxRows
	} else {
		// Not logged in, so use the default number of rows
		tempMaxRows = com.DefaultNumDisplayRows
		pageData.DB.MaxRows = tempMaxRows
	}

	// Generate predictable cache keys for the metadata and sqlite table rows
	mdataCacheKey := com.MetadataCacheKey("dwndb-meta", loggedInUser, dbOwner, dbFolder, dbName,
		commitID)
	rowCacheKey := com.TableRowsCacheKey(fmt.Sprintf("tablejson/%s/%s/%d", sortCol, sortDir, rowOffset),
		loggedInUser, dbOwner, dbFolder, dbName, commitID, dbTable, pageData.DB.MaxRows)

	// If a cached version of the page data exists, use it
	ok, err := com.GetCachedData(mdataCacheKey, &pageData)
	if err != nil {
		log.Printf("%s: Error retrieving page data from cache: %v\n", pageName, err)
	}
	if ok {
		// Grab the cached table data as well
		ok, err := com.GetCachedData(rowCacheKey, &pageData.Data)
		if err != nil {
			log.Printf("%s: Error retrieving page data from cache: %v\n", pageName, err)
		}

		// Restore the correct MaxRow value
		pageData.DB.MaxRows = tempMaxRows

		// Restore the correct username
		pageData.Meta.LoggedInUser = loggedInUser

		// Get latest star and fork count
		_, pageData.DB.Info.Stars, pageData.DB.Info.Forks, err = com.SocialStats(dbOwner, dbFolder, dbName)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, err.Error())
			return
		}

		// Render the page (using the caches)
		if ok {
			t := tmpl.Lookup("databasePage")
			err = t.Execute(w, pageData)
			if err != nil {
				log.Printf("Error: %s", err)
			}
			return
		}

		// Note - If the row data wasn't found in cache, we fall through and continue on with the rest of this
		//        function, which grabs it and caches it for future use
	}

	// Get a handle from Minio for the database object
	sdb, err := com.OpenMinioObject(pageData.DB.Info.DBEntry.Sha256[:com.MinioFolderChars],
		pageData.DB.Info.DBEntry.Sha256[com.MinioFolderChars:])
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// Close the SQLite database and delete the temp file
	defer func() {
		sdb.Close()
	}()

	// Retrieve the list of tables in the database
	tables, err := com.Tables(sdb, dbName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	pageData.DB.Info.Tables = tables

	// If a specific table was requested, check that it's present
	if dbTable != "" {
		// Check the requested table is present
		tablePresent := false
		for _, tbl := range tables {
			if tbl == dbTable {
				tablePresent = true
			}
		}
		if tablePresent == false {
			// The requested table doesn't exist in the database
			log.Printf("%s: Requested table not present in database. DB: '%s%s%s', Table: '%s'\n",
				pageName, dbOwner, dbFolder, dbName, dbTable)
			errorPage(w, r, http.StatusBadRequest, "Requested table not present")
			return
		}
	}

	// If a specific table wasn't requested, use the first table in the database that passes validation
	if dbTable == "" {
		for _, i := range pageData.DB.Info.Tables {
			if i != "" {
				err = com.ValidatePGTable(i)
				if err == nil {
					// The database table name is acceptable, so use it
					dbTable = i
					break
				}
			}
		}
	}

	// If a sort column was requested, verify it exists
	if sortCol != "" {
		colList, err := sdb.Columns("", dbTable)
		if err != nil {
			log.Printf("Error when reading column names for table '%s': %v\n", dbTable,
				err.Error())
			errorPage(w, r, http.StatusInternalServerError, "Error when reading from the database")
			return
		}
		colExists := false
		for _, j := range colList {
			if j.Name == sortCol {
				colExists = true
			}
		}
		if colExists == false {
			// The requested sort column doesn't exist, so we fall back to no sorting
			sortCol = ""
		}
	}

	// Validate the table name, just to be careful
	if dbTable != "" {
		err = com.ValidatePGTable(dbTable)
		if err != nil {
			// Validation failed, so don't pass on the table name

			// If the failed table name is "{{ db.Tablename }}", don't bother logging it.  It's just a search
			// bot picking up AngularJS in a string and doing a request with it
			if dbTable != "{{ db.Tablename }}" {
				log.Printf("%s: Validation failed for table name: '%s': %s", pageName, dbTable, err)
			}
			errorPage(w, r, http.StatusBadRequest, "Validation failed for table name")
			return
		}
	}

	// Fill out various metadata fields
	pageData.Meta.Owner = dbOwner
	pageData.Meta.Database = dbName
	pageData.Meta.Server = com.Conf.Web.ServerName
	pageData.Meta.Title = fmt.Sprintf("%s %s %s", dbOwner, dbFolder, dbName)

	// Fill out the branch info
	for i := range branchHeads {
		pageData.DB.Info.BranchList = append(pageData.DB.Info.BranchList, i)
	}
	if branchName == "" {
		branchName, err = com.GetDefaultBranchName(dbOwner, dbFolder, dbName)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, "Error retrieving default branch name")
			return
		}
	}
	pageData.DB.Info.Branch = branchName
	pageData.DB.Info.Commits = branchHeads[branchName].CommitCount

	// Retrieve the "forked from" information
	frkOwn, frkFol, frkDB, frkDel, err := com.ForkedFrom(dbOwner, dbFolder, dbName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, "Database query failure")
		return
	}
	pageData.Meta.ForkOwner = frkOwn
	pageData.Meta.ForkFolder = frkFol
	pageData.Meta.ForkDatabase = frkDB
	pageData.Meta.ForkDeleted = frkDel

	// Add Auth0 info to the page data
	pageData.Auth0.CallbackURL = "https://" + com.Conf.Web.ServerName + "/x/callback"
	pageData.Auth0.ClientID = com.Conf.Auth0.ClientID
	pageData.Auth0.Domain = com.Conf.Auth0.Domain

	// Update database star status for the logged in user
	pageData.MyStar = myStar

	// Render the full description as markdown
	pageData.DB.Info.FullDesc = string(gfm.Markdown([]byte(pageData.DB.Info.FullDesc)))

	// Cache the page metadata
	err = com.CacheData(mdataCacheKey, pageData, com.Conf.Memcache.DefaultCacheTime)
	if err != nil {
		log.Printf("%s: Error when caching page data: %v\n", pageName, err)
	}

	// Grab the cached table data if it's available
	ok, err = com.GetCachedData(rowCacheKey, &pageData.Data)
	if err != nil {
		log.Printf("%s: Error retrieving page data from cache: %v\n", pageName, err)
	}

	// If the row data wasn't in cache, read it from the database
	if !ok {
		pageData.Data, err = com.ReadSQLiteDB(sdb, dbTable, pageData.DB.MaxRows, sortCol, sortDir, rowOffset)
		if err != nil {
			// Some kind of error when reading the database data
			errorPage(w, r, http.StatusBadRequest, err.Error())
			return
		}
		pageData.Data.Tablename = dbTable
	}

	// Cache the table row data
	err = com.CacheData(rowCacheKey, pageData.Data, com.Conf.Memcache.DefaultCacheTime)
	if err != nil {
		log.Printf("%s: Error when caching page data: %v\n", pageName, err)
	}

	// Render the page
	t := tmpl.Lookup("databasePage")
	err = t.Execute(w, pageData)
	if err != nil {
		log.Printf("Error: %s", err)
	}
}

func discussPage(w http.ResponseWriter, r *http.Request) {
	var pageData struct {
		Auth0          com.Auth0Set
		CommentList    []com.DiscussionCommentEntry
		DB             com.SQLiteDBinfo
		DiscussionList []com.DiscussionEntry
		Meta           com.MetaInfo
		SelectedID     int
		MyStar         bool
	}

	// Retrieve session data (if any)
	var loggedInUser string
	sess, err := store.Get(r, "dbhub-user")
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}
	u := sess.Values["UserName"]
	if u != nil {
		loggedInUser = u.(string)
		pageData.Meta.LoggedInUser = loggedInUser
	}

	// Retrieve the database owner & name
	// TODO: Add folder support
	dbFolder := "/"
	dbOwner, dbName, err := com.GetOD(1, r) // 1 = Ignore "/discuss/" at the start of the URL
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Check if a discussion id was provided
	a := r.FormValue("id")                   // Optional
	if a != "" && a != "{{ row.disc_id }}" { // Search engines have a habit of passing AngularJS tags, so we ignore when the field has the AngularJS tag in it
		pageData.SelectedID, err = strconv.Atoi(a)
		if err != nil {
			log.Printf("Error converting string '%s' to integer in function '%s': %s\n", a,
				com.GetCurrentFunctionName(), err)
			errorPage(w, r, http.StatusBadRequest, "Error when parsing discussion id value")
			return
		}
	}

	// Validate the supplied information
	if dbOwner == "" || dbName == "" {
		errorPage(w, r, http.StatusBadRequest, "Missing database owner or database name")
		return
	}

	// Check if the requested database exists
	exists, err := com.CheckDBExists(loggedInUser, dbOwner, dbFolder, dbName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	if !exists {
		errorPage(w, r, http.StatusNotFound, fmt.Sprintf("Database '%s%s%s' doesn't exist", dbOwner, dbFolder,
			dbName))
		return
	}

	// Check if the user has access to the requested database (and get it's details if available)
	err = com.DBDetails(&pageData.DB, loggedInUser, dbOwner, dbFolder, dbName, "")
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Get latest star and fork count
	_, pageData.DB.Info.Stars, pageData.DB.Info.Forks, err = com.SocialStats(dbOwner, dbFolder, dbName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// Check if the database was starred by the logged in user
	pageData.MyStar, err = com.CheckDBStarred(loggedInUser, dbOwner, dbFolder, dbName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, "Couldn't retrieve latest social stats")
		return
	}

	// Retrieve the list of discussions for this database
	pageData.DiscussionList, err = com.Discussions(dbOwner, dbFolder, dbName, pageData.SelectedID)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// Fill out the metadata
	pageData.Meta.Owner = dbOwner
	pageData.Meta.Database = dbName

	// Add Auth0 info to the page data
	pageData.Auth0.CallbackURL = "https://" + com.Conf.Web.ServerName + "/x/callback"
	pageData.Auth0.ClientID = com.Conf.Auth0.ClientID
	pageData.Auth0.Domain = com.Conf.Auth0.Domain

	// If a specific discussion ID was given, then we display the discussion comments page
	if pageData.SelectedID != 0 {
		// Load the comments for the requested discussion
		pageData.CommentList, err = com.DiscussionComments(dbOwner, dbFolder, dbName, pageData.SelectedID, 0)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, err.Error())
			return
		}

		// Render the discussion comments page
		t := tmpl.Lookup("discussCommentsPage")
		err = t.Execute(w, pageData)
		if err != nil {
			log.Printf("Error: %s", err)
		}
		return
	}

	// Render the main discussion list page
	t := tmpl.Lookup("discussListPage")
	err = t.Execute(w, pageData)
	if err != nil {
		log.Printf("Error: %s", err)
	}
}

// General error display page.
func errorPage(w http.ResponseWriter, r *http.Request, httpCode int, msg string) {
	var pageData struct {
		Auth0   com.Auth0Set
		Message string
		Meta    com.MetaInfo
	}
	pageData.Message = msg

	// Retrieve session data (if any)
	var loggedInUser string
	sess, err := store.Get(r, "dbhub-user")
	if err != nil {
		fmt.Fprintf(w, "An error occurred when calling errorPage(): %s", err.Error())
		return
	}
	u := sess.Values["UserName"]
	if u != nil {
		loggedInUser = u.(string)
		pageData.Meta.LoggedInUser = loggedInUser
	}

	// Add Auth0 info to the page data
	pageData.Auth0.CallbackURL = "https://" + com.Conf.Web.ServerName + "/x/callback"
	pageData.Auth0.ClientID = com.Conf.Auth0.ClientID
	pageData.Auth0.Domain = com.Conf.Auth0.Domain

	// Render the page
	w.WriteHeader(httpCode)
	t := tmpl.Lookup("errorPage")
	err = t.Execute(w, pageData)
	if err != nil {
		log.Printf("Error: %s", err)
	}
}

// Render the page showing forks of the given database
func forksPage(w http.ResponseWriter, r *http.Request) {
	var pageData struct {
		Auth0 com.Auth0Set
		Meta  com.MetaInfo
		Forks []com.ForkEntry
	}
	pageData.Meta.Title = "Forks"

	// Retrieve user and database name
	dbOwner, dbName, err := com.GetOD(1, r) // 1 = Ignore "/forks/" at the start of the URL
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}
	pageData.Meta.Owner = dbOwner
	pageData.Meta.Database = dbName
	dbFolder := "/"

	// Retrieve session data (if any)
	var loggedInUser string
	sess, err := store.Get(r, "dbhub-user")
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}
	u := sess.Values["UserName"]
	if u != nil {
		loggedInUser = u.(string)
		pageData.Meta.LoggedInUser = loggedInUser
	}

	// Check if the database exists
	exists, err := com.CheckDBExists(loggedInUser, dbOwner, dbFolder, dbName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, "Database failure when looking up database details")
		return
	}
	if !exists {
		errorPage(w, r, http.StatusNotFound, "That database doesn't seem to exist")
		return
	}

	// Retrieve list of forks for the database
	pageData.Forks, err = com.ForkTree(loggedInUser, dbOwner, dbFolder, dbName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError,
			fmt.Sprintf("Error retrieving fork list for '%s%s%s': %v\n", dbOwner, dbFolder,
				dbName, err.Error()))
		return
	}

	// Add Auth0 info to the page data
	pageData.Auth0.CallbackURL = "https://" + com.Conf.Web.ServerName + "/x/callback"
	pageData.Auth0.ClientID = com.Conf.Auth0.ClientID
	pageData.Auth0.Domain = com.Conf.Auth0.Domain

	// Render the page
	t := tmpl.Lookup("forksPage")
	err = t.Execute(w, pageData)
	if err != nil {
		log.Printf("Error: %s", err)
	}
}

// Renders the front page of the website.
func frontPage(w http.ResponseWriter, r *http.Request) {
	// Structure to hold page data
	var pageData struct {
		Auth0 com.Auth0Set
		Meta  com.MetaInfo
		Stats map[com.ActivityRange]com.ActivityStats
	}

	// Retrieve session data (if any)
	var loggedInUser string
	sess, err := store.Get(r, "dbhub-user")
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}
	u := sess.Values["UserName"]
	if u != nil {
		loggedInUser = u.(string)
		pageData.Meta.LoggedInUser = loggedInUser
	}

	// Retrieve the list of most forked and starred databases
	pageData.Stats = make(map[com.ActivityRange]com.ActivityStats)
	statsAll, err := com.GetActivityStats()
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	pageData.Stats[com.ALL_TIME] = statsAll

	// Set other relevant metadata
	pageData.Meta.Title = `SQLite storage "in the cloud"`

	// Add Auth0 info to the page data
	pageData.Auth0.CallbackURL = "https://" + com.Conf.Web.ServerName + "/x/callback"
	pageData.Auth0.ClientID = com.Conf.Auth0.ClientID
	pageData.Auth0.Domain = com.Conf.Auth0.Domain

	// Render the page
	t := tmpl.Lookup("rootPage")
	err = t.Execute(w, pageData)
	if err != nil {
		log.Printf("Error: %s", err)
	}
}

// Renders the user Preferences page.
func prefPage(w http.ResponseWriter, r *http.Request, loggedInUser string) {
	var pageData struct {
		Auth0       com.Auth0Set
		DisplayName string
		Email       string
		MaxRows     int
		Meta        com.MetaInfo
	}
	pageData.Meta.Title = "Preferences"
	pageData.Meta.LoggedInUser = loggedInUser

	// Grab the user's display name and email address
	var err error
	pageData.DisplayName, pageData.Email, err = com.GetUserDetails(loggedInUser)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, "Database query failed")
		return
	}

	// Set the server name, used for the placeholder email address suggestion
	serverName := strings.Split(com.Conf.Web.ServerName, ":")
	pageData.Meta.Server = serverName[0]

	// If the user's email address is empty, use username@server by default.  This mirrors the suggestion on the
	// rendered HTML, so the user doesn't have to manually type it in
	if pageData.Email == "" {
		pageData.Email = fmt.Sprintf("%s@%s", loggedInUser, pageData.Meta.Server)
	}

	// Retrieve the user preference data
	pageData.MaxRows = com.PrefUserMaxRows(loggedInUser)

	// Add Auth0 info to the page data
	pageData.Auth0.CallbackURL = "https://" + com.Conf.Web.ServerName + "/x/callback"
	pageData.Auth0.ClientID = com.Conf.Auth0.ClientID
	pageData.Auth0.Domain = com.Conf.Auth0.Domain

	// Render the page
	t := tmpl.Lookup("prefPage")
	err = t.Execute(w, pageData)
	if err != nil {
		log.Printf("Error: %s", err)
	}
}

func profilePage(w http.ResponseWriter, r *http.Request, userName string) {
	var pageData struct {
		Auth0      com.Auth0Set
		Meta       com.MetaInfo
		PrivateDBs []com.DBInfo
		PublicDBs  []com.DBInfo
		Stars      []com.DBEntry
	}
	pageData.Meta.Owner = userName
	pageData.Meta.Title = userName
	pageData.Meta.Server = com.Conf.Web.ServerName
	pageData.Meta.LoggedInUser = userName

	// Check if the desired user exists
	userExists, err := com.CheckUserExists(userName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, "Database query failed")
		return
	}

	// If the user doesn't exist, indicate that
	if !userExists {
		errorPage(w, r, http.StatusNotFound, fmt.Sprintf("Unknown user: %s", userName))
		return
	}

	// Retrieve list of public databases for the user
	pageData.PublicDBs, err = com.UserDBs(userName, com.DB_PUBLIC)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, "Database query failed")
		return
	}

	// Retrieve list of private databases for the user
	pageData.PrivateDBs, err = com.UserDBs(userName, com.DB_PRIVATE)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, "Database query failed")
		return
	}

	// Retrieve the list of starred databases for the user
	pageData.Stars, err = com.UserStarredDBs(userName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, "Database query failed")
		return
	}

	// Add Auth0 info to the page data
	pageData.Auth0.CallbackURL = "https://" + com.Conf.Web.ServerName + "/x/callback"
	pageData.Auth0.ClientID = com.Conf.Auth0.ClientID
	pageData.Auth0.Domain = com.Conf.Auth0.Domain

	// Render the page
	t := tmpl.Lookup("profilePage")
	err = t.Execute(w, pageData)
	if err != nil {
		log.Printf("Error: %s", err)
	}
}

// Render the releases page, which displays the releases for a database.
func releasesPage(w http.ResponseWriter, r *http.Request) {
	// Structure to hold page data
	type relEntry struct {
		Commit              string    `json:"commit"`
		Date                time.Time `json:"date"`
		Description         string    `json:"description"`
		DescriptionMarkdown string    `json:"description_markdown"`
		ReleaserUserName    string    `json:"releaser_user_name"`
		ReleaserDisplayName string    `json:"releaser_display_name"`
		Size                int       `json:"size"`
	}
	var pageData struct {
		Auth0       com.Auth0Set
		DB          com.SQLiteDBinfo
		Meta        com.MetaInfo
		ReleaseList map[string]relEntry
		NumRels     int
	}
	pageData.Meta.Title = "Release list"

	// Retrieve session data (if any)
	var loggedInUser string
	sess, err := store.Get(r, "dbhub-user")
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}
	u := sess.Values["UserName"]
	if u != nil {
		loggedInUser = u.(string)
		pageData.Meta.LoggedInUser = loggedInUser
	}

	// Retrieve the database owner & name
	// TODO: Add folder support
	dbFolder := "/"
	dbOwner, dbName, err := com.GetOD(1, r) // 1 = Ignore "/releases/" at the start of the URL
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Validate the supplied information
	if dbOwner == "" || dbName == "" {
		errorPage(w, r, http.StatusBadRequest, "Missing database owner or database name")
		return
	}

	// Check if the requested database exists
	exists, err := com.CheckDBExists(loggedInUser, dbOwner, dbFolder, dbName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	if !exists {
		errorPage(w, r, http.StatusNotFound, fmt.Sprintf("Database '%s%s%s' doesn't exist", dbOwner, dbFolder,
			dbName))
		return
	}

	// Check if the user has access to the requested database (and get it's details if available)
	err = com.DBDetails(&pageData.DB, loggedInUser, dbOwner, dbFolder, dbName, "")
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Retrieve the release list for the database
	releases, err := com.GetReleases(dbOwner, dbFolder, dbName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// Create a small username/email lookup cache, so we don't have to query the database for usernames we've already
	// looked up
	userNameCache := make(map[string]string)

	// Fill out the metadata
	pageData.Meta.Owner = dbOwner
	pageData.Meta.Database = dbName
	pageData.ReleaseList = make(map[string]relEntry)
	pageData.NumRels = len(releases)
	if pageData.NumRels > 0 {
		for i, j := range releases {
			// If the username/email address entry is already in the username cache then use it, else grab it from the
			// database (and put it in the cache)
			_, ok := userNameCache[j.ReleaserEmail]
			if !ok {
				userNameCache[j.ReleaserEmail], err = com.GetUsernameFromEmail(j.ReleaserEmail)
				if err != nil {
					errorPage(w, r, http.StatusInternalServerError, err.Error())
					return
				}
			}

			// Create the tag info we pass to the tag list rendering page
			pageData.ReleaseList[i] = relEntry{
				Commit:              j.Commit,
				Date:                j.Date,
				Description:         j.Description,
				DescriptionMarkdown: string(gfm.Markdown([]byte(j.Description))),
				ReleaserUserName:    userNameCache[j.ReleaserEmail],
				ReleaserDisplayName: j.ReleaserName,
				Size:                j.Size,
			}
		}
	}

	// Add Auth0 info to the page data
	pageData.Auth0.CallbackURL = "https://" + com.Conf.Web.ServerName + "/x/callback"
	pageData.Auth0.ClientID = com.Conf.Auth0.ClientID
	pageData.Auth0.Domain = com.Conf.Auth0.Domain

	// Render the page
	t := tmpl.Lookup("releasesPage")
	err = t.Execute(w, pageData)
	if err != nil {
		log.Printf("Error: %s", err)
	}
}

// Displays a web page for new users to choose their username.
func selectUserNamePage(w http.ResponseWriter, r *http.Request) {
	var pageData struct {
		Auth0 com.Auth0Set
		Meta  com.MetaInfo
		Nick  string
	}
	pageData.Meta.Title = "Select your username"

	// Retrieve session data (if any)
	sess, err := store.Get(r, "user-reg")
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}
	if sess.IsNew {
		// This isn't a valid username selection session, so abort
		errorPage(w, r, http.StatusBadRequest, "Invalid user creation session")
		return
	}
	validRegSession := false
	rip := sess.Values["registrationinprogress"]
	if rip == nil {
		// This isn't a valid username selection session, so abort
		errorPage(w, r, http.StatusBadRequest, "Invalid user creation session")
		return
	}
	validRegSession = rip.(bool)
	if validRegSession != true {
		// For some reason this isn't a valid username selection session, so abort
		errorPage(w, r, http.StatusBadRequest, "Invalid user creation session")
		return
	}

	// Add Auth0 info to the page data
	pageData.Auth0.CallbackURL = "https://" + com.Conf.Web.ServerName + "/x/callback"
	pageData.Auth0.ClientID = com.Conf.Auth0.ClientID
	pageData.Auth0.Domain = com.Conf.Auth0.Domain

	// If the Auth0 profile included a nickname, we use that to pre-fill the input field
	ni := sess.Values["nickname"]
	if ni != nil {
		pageData.Nick = ni.(string)
	}

	// Render the page
	t := tmpl.Lookup("selectUserNamePage")
	err = t.Execute(w, pageData)
	if err != nil {
		log.Printf("Error: %s", err)
	}
}

// Render the settings page.
func settingsPage(w http.ResponseWriter, r *http.Request) {
	// Structures to hold page data
	var pageData struct {
		Auth0       com.Auth0Set
		BranchLics  map[string]string
		DB          com.SQLiteDBinfo
		Licences    map[string]com.LicenceEntry
		Meta        com.MetaInfo
		NumLicences int
	}
	pageData.Meta.Title = "Database settings"

	// Retrieve session data (if any)
	var loggedInUser string
	validSession := false
	sess, err := store.Get(r, "dbhub-user")
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}
	u := sess.Values["UserName"]
	if u != nil {
		loggedInUser = u.(string)
		pageData.Meta.LoggedInUser = loggedInUser
		validSession = true
	}

	// Ensure we have a valid logged in user
	if validSession != true {
		errorPage(w, r, http.StatusUnauthorized, "You need to be logged in")
		return
	}

	// Retrieve the database owner, database name
	// TODO: Add folder support
	dbFolder := "/"
	dbOwner, dbName, err := com.GetOD(1, r) // 1 = Ignore "/settings/" at the start of the URL
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Validate the supplied information
	if dbOwner == "" || dbName == "" {
		errorPage(w, r, http.StatusBadRequest, "Missing database owner or database name")
		return
	}
	if dbOwner != loggedInUser {
		errorPage(w, r, http.StatusBadRequest,
			"You can only access the settings page for your own databases")
		return
	}

	// Check if the user has access to the requested database (and get it's details if available)
	err = com.DBDetails(&pageData.DB, loggedInUser, dbOwner, dbFolder, dbName, "")
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Get a handle from Minio for the database object
	bkt := pageData.DB.Info.DBEntry.Sha256[:com.MinioFolderChars]
	id := pageData.DB.Info.DBEntry.Sha256[com.MinioFolderChars:]
	sdb, err := com.OpenMinioObject(bkt, id)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// Automatically close the SQLite database when this function finishes
	defer func() {
		sdb.Close()
	}()

	// Retrieve the list of tables in the database
	pageData.DB.Info.Tables, err = com.Tables(sdb, fmt.Sprintf("%s%s%s", dbOwner, dbFolder, dbName))
	defer sdb.Close()
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// Retrieve the list of branches
	branchHeads, err := com.GetBranches(dbOwner, dbFolder, dbName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// Retrieve all of the commits for the database
	commitList, err := com.GetCommitList(dbOwner, dbFolder, dbName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// Work out the licence assigned to each of the branch heads
	pageData.BranchLics = make(map[string]string)
	for bName, bEntry := range branchHeads {
		c, ok := commitList[bEntry.Commit]
		if !ok {
			errorPage(w, r, http.StatusInternalServerError, fmt.Sprintf(
				"Couldn't retrieve branch '%s' head commit '%s' for database '%s%s%s'\n", bName, bEntry.Commit,
				dbOwner, dbFolder, dbName))
			return
		}
		licSHA := c.Tree.Entries[0].LicenceSHA

		// If the licence SHA256 field isn't empty, look up the licence info corresponding to it
		var a string
		if licSHA != "" {
			a, _, err = com.GetLicenceInfoFromSha256(dbOwner, licSHA)
			if err != nil {
				errorPage(w, r, http.StatusInternalServerError, err.Error())
				return
			}
		} else {
			a = "Not specified"
		}
		pageData.BranchLics[bName] = a
	}

	// Populate the licence list
	pageData.Licences, err = com.GetLicences(loggedInUser)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, "Error when retrieving list of available licences")
		return
	}
	pageData.NumLicences = len(pageData.Licences)

	// Fill out the metadata
	pageData.Meta.Owner = dbOwner
	pageData.Meta.Database = dbName

	// If the default table is blank, use the first one from the table list
	if pageData.DB.Info.DefaultTable == "" {
		pageData.DB.Info.DefaultTable = pageData.DB.Info.Tables[0]
	}

	// Add Auth0 info to the page data
	pageData.Auth0.CallbackURL = "https://" + com.Conf.Web.ServerName + "/x/callback"
	pageData.Auth0.ClientID = com.Conf.Auth0.ClientID
	pageData.Auth0.Domain = com.Conf.Auth0.Domain

	// Render the page
	t := tmpl.Lookup("settingsPage")
	err = t.Execute(w, pageData)
	if err != nil {
		log.Printf("Error: %s", err)
	}
}

// Present the stars page to the user.
func starsPage(w http.ResponseWriter, r *http.Request) {
	var pageData struct {
		Auth0 com.Auth0Set
		Meta  com.MetaInfo
		Stars []com.DBEntry
	}
	pageData.Meta.Title = "Stars"

	// Retrieve session data (if any)
	var loggedInUser string
	sess, err := store.Get(r, "dbhub-user")
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}
	u := sess.Values["UserName"]
	if u != nil {
		loggedInUser = u.(string)
		pageData.Meta.LoggedInUser = loggedInUser
	}

	// Retrieve owner and database name
	dbOwner, dbName, err := com.GetOD(1, r) // 1 = Ignore "/stars/" at the start of the URL
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}
	pageData.Meta.Owner = dbOwner
	pageData.Meta.Database = dbName

	// Check if the database exists
	// TODO: Add folder support
	dbFolder := "/"
	exists, err := com.CheckDBExists(loggedInUser, dbOwner, dbFolder, dbName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, "Database failure when looking up database details")
		return
	}
	if !exists {
		errorPage(w, r, http.StatusNotFound, "That database doesn't seem to exist")
		return
	}

	// Retrieve list of users who starred the database
	pageData.Stars, err = com.UsersStarredDB(dbOwner, dbFolder, dbName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, "Database query failed")
		return
	}

	// Add Auth0 info to the page data
	pageData.Auth0.CallbackURL = "https://" + com.Conf.Web.ServerName + "/x/callback"
	pageData.Auth0.ClientID = com.Conf.Auth0.ClientID
	pageData.Auth0.Domain = com.Conf.Auth0.Domain

	// Render the page
	t := tmpl.Lookup("starsPage")
	err = t.Execute(w, pageData)
	if err != nil {
		log.Printf("Error: %s", err)
	}
}

// Render the tag page, which displays the tags for a database.
func tagsPage(w http.ResponseWriter, r *http.Request) {
	// Structure to hold page data
	type tgEntry struct {
		Commit              string    `json:"commit"`
		Date                time.Time `json:"date"`
		Description         string    `json:"description"`
		DescriptionMarkdown string    `json:"description_markdown"`
		TaggerUserName      string    `json:"tagger_user_name"`
		TaggerDisplayName   string    `json:"tagger_display_name"`
	}
	var pageData struct {
		Auth0   com.Auth0Set
		DB      com.SQLiteDBinfo
		Meta    com.MetaInfo
		TagList map[string]tgEntry
		NumTags int
	}
	pageData.Meta.Title = "Tag list"

	// Retrieve session data (if any)
	var loggedInUser string
	sess, err := store.Get(r, "dbhub-user")
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}
	u := sess.Values["UserName"]
	if u != nil {
		loggedInUser = u.(string)
		pageData.Meta.LoggedInUser = loggedInUser
	}

	// Retrieve the database owner & name
	// TODO: Add folder support
	dbFolder := "/"
	dbOwner, dbName, err := com.GetOD(1, r) // 1 = Ignore "/tags/" at the start of the URL
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Validate the supplied information
	if dbOwner == "" || dbName == "" {
		errorPage(w, r, http.StatusBadRequest, "Missing database owner or database name")
		return
	}

	// Check if the requested database exists
	exists, err := com.CheckDBExists(loggedInUser, dbOwner, dbFolder, dbName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	if !exists {
		errorPage(w, r, http.StatusNotFound, fmt.Sprintf("Database '%s%s%s' doesn't exist", dbOwner, dbFolder,
			dbName))
		return
	}

	// Check if the user has access to the requested database (and get it's details if available)
	err = com.DBDetails(&pageData.DB, loggedInUser, dbOwner, dbFolder, dbName, "")
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Retrieve the tag list for the database
	tags, err := com.GetTags(dbOwner, dbFolder, dbName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// Create a small username/email lookup cache, so we don't have to query the database for usernames we've already
	// looked up
	userNameCache := make(map[string]string)

	// Fill out the metadata
	pageData.Meta.Owner = dbOwner
	pageData.Meta.Database = dbName
	pageData.TagList = make(map[string]tgEntry)
	pageData.NumTags = len(tags)
	if pageData.NumTags > 0 {
		for i, j := range tags {
			// If the username/email address entry is already in the username cache then use it, else grab it from the
			// database (and put it in the cache)
			_, ok := userNameCache[j.TaggerEmail]
			if !ok {
				userNameCache[j.TaggerEmail], err = com.GetUsernameFromEmail(j.TaggerEmail)
				if err != nil {
					errorPage(w, r, http.StatusInternalServerError, err.Error())
					return
				}
			}

			// Create the tag info we pass to the tag list rendering page
			pageData.TagList[i] = tgEntry{
				Commit:              j.Commit,
				Date:                j.Date,
				Description:         j.Description,
				DescriptionMarkdown: string(gfm.Markdown([]byte(j.Description))),
				TaggerUserName:      userNameCache[j.TaggerEmail],
				TaggerDisplayName:   j.TaggerName,
			}
		}
	}

	// Add Auth0 info to the page data
	pageData.Auth0.CallbackURL = "https://" + com.Conf.Web.ServerName + "/x/callback"
	pageData.Auth0.ClientID = com.Conf.Auth0.ClientID
	pageData.Auth0.Domain = com.Conf.Auth0.Domain

	// Render the page
	t := tmpl.Lookup("tagsPage")
	err = t.Execute(w, pageData)
	if err != nil {
		log.Printf("Error: %s", err)
	}
}

// This function presents the database upload form to logged in users.
func uploadPage(w http.ResponseWriter, r *http.Request) {
	// Data to pass to the upload form
	var pageData struct {
		Auth0         com.Auth0Set
		Branches      []string
		DefaultBranch string
		Licences      map[string]com.LicenceEntry
		Meta          com.MetaInfo
		NumLicences   int
	}

	// Retrieve session data (if any)
	var loggedInUser string
	validSession := false
	sess, err := store.Get(r, "dbhub-user")
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}
	u := sess.Values["UserName"]
	if u != nil {
		loggedInUser = u.(string)
		pageData.Meta.LoggedInUser = loggedInUser
		validSession = true
	}

	// Ensure we have a valid logged in user
	if validSession != true {
		errorPage(w, r, http.StatusUnauthorized, "You need to be logged in")
		return
	}

	// Ensure the user has set their display name and email address
	displayName, email, err := com.GetUserDetails(loggedInUser)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, "Error when retrieving user details")
		return
	}
	if displayName == "" || email == "" {
		errorPage(w, r, http.StatusBadRequest,
			"You need to set your full name and email address in Preferences first")
		return
	}

	// Populate the licence list
	pageData.Licences, err = com.GetLicences(loggedInUser)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, "Error when retrieving list of available licences")
		return
	}
	pageData.NumLicences = len(pageData.Licences)

	// Fill out page metadata
	pageData.Meta.Title = "Upload database"
	pageData.Meta.LoggedInUser = loggedInUser

	// Add Auth0 info to the page data
	pageData.Auth0.CallbackURL = "https://" + com.Conf.Web.ServerName + "/x/callback"
	pageData.Auth0.ClientID = com.Conf.Auth0.ClientID
	pageData.Auth0.Domain = com.Conf.Auth0.Domain

	// Render the page
	t := tmpl.Lookup("uploadPage")
	err = t.Execute(w, pageData)
	if err != nil {
		log.Printf("Error: %s", err)
	}
}

func userPage(w http.ResponseWriter, r *http.Request, userName string) {
	// Structure to hold page data
	var pageData struct {
		Auth0    com.Auth0Set
		DBRows   []com.DBInfo
		FullName string
		Meta     com.MetaInfo
	}
	pageData.Meta.Owner = userName
	pageData.Meta.Title = userName
	pageData.Meta.Server = com.Conf.Web.ServerName

	// Retrieve session data (if any)
	var loggedInUser string
	sess, err := store.Get(r, "dbhub-user")
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}
	u := sess.Values["UserName"]
	if u != nil {
		loggedInUser = u.(string)
		if loggedInUser == userName {
			// The logged in user is looking at their own user page
			profilePage(w, r, loggedInUser)
			return
		}
		pageData.Meta.LoggedInUser = loggedInUser
	}

	// Check if the desired user exists
	userExists, err := com.CheckUserExists(userName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, "Database query failed")
		return
	}

	// If the user doesn't exist, indicate that
	if !userExists {
		errorPage(w, r, http.StatusNotFound, fmt.Sprintf("Unknown user: %s", userName))
		return
	}

	// Retrieve the full name for the user
	pageData.FullName, _, err = com.GetUserDetails(userName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// Retrieve list of public databases for the user
	pageData.DBRows, err = com.UserDBs(userName, com.DB_PUBLIC)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, "Database query failed")
		return
	}

	// Add Auth0 info to the page data
	pageData.Auth0.CallbackURL = "https://" + com.Conf.Web.ServerName + "/x/callback"
	pageData.Auth0.ClientID = com.Conf.Auth0.ClientID
	pageData.Auth0.Domain = com.Conf.Auth0.Domain

	// Render the page
	t := tmpl.Lookup("userPage")
	err = t.Execute(w, pageData)
	if err != nil {
		log.Printf("Error: %s", err)
	}
}
