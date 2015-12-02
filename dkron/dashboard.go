package dkron

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"path/filepath"
	"sort"

	"github.com/gorilla/mux"
)

const (
	tmplPath            = "templates"
	dashboardPathPrefix = "dashboard"
	apiPathPrefix       = "v1"
)

type int64arr []int64

func (a int64arr) Len() int           { return len(a) }
func (a int64arr) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a int64arr) Less(i, j int) bool { return a[i] < a[j] }

type commonDashboardData struct {
	Version    string
	LeaderName string
	MemberName string
	Backend    string
	Path       string
	APIPath    string
	Keyspace   string
}

func newCommonDashboardData(a *AgentCommand, nodeName, path string) *commonDashboardData {
	l, _ := a.leaderMember()
	return &commonDashboardData{
		Version:    a.Version,
		LeaderName: l.Name,
		MemberName: nodeName,
		Backend:    a.config.Backend,
		Path:       fmt.Sprintf("%s%s", path, dashboardPathPrefix),
		APIPath:    fmt.Sprintf("%s%s", path, apiPathPrefix),
		Keyspace:   a.config.Keyspace,
	}
}

func (a *AgentCommand) dashboardRoutes(r *mux.Router) {
	r.Path("/" + dashboardPathPrefix).HandlerFunc(a.dashboardIndexHandler).Methods("GET")
	subui := r.PathPrefix("/" + dashboardPathPrefix).Subrouter()
	subui.HandleFunc("/jobs", a.dashboardJobsHandler).Methods("GET")
	subui.HandleFunc("/jobs/{job}/executions", a.dashboardExecutionsHandler).Methods("GET")

	// Path of static files must be last!
	r.PathPrefix("/dashboard").Handler(
		http.StripPrefix("/dashboard", http.FileServer(
			http.Dir(filepath.Join(a.config.UIDir, "static")))))
	r.PathPrefix("/").Handler(http.RedirectHandler("dashboard", 301))
}

func templateSet(uiDir string, template string) []string {
	return []string{
		filepath.Join(uiDir, tmplPath, "dashboard.html.tmpl"),
		filepath.Join(uiDir, tmplPath, "status.html.tmpl"),
		filepath.Join(uiDir, tmplPath, template+".html.tmpl"),
	}
}

func (a *AgentCommand) dashboardIndexHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")

	tmpl := template.Must(template.New("dashboard.html.tmpl").ParseFiles(
		templateSet(a.config.UIDir, "index")...))

	data := struct {
		Common *commonDashboardData
	}{
		Common: newCommonDashboardData(a, a.config.NodeName, ""),
	}

	if err := tmpl.Execute(w, data); err != nil {
		log.Error(err)
	}
}

func (a *AgentCommand) dashboardJobsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")

	jobs, _ := a.store.GetJobs()

	funcs := template.FuncMap{
		"executionStatus": func(job *Job) string {
			execs, _ := a.store.GetLastExecutionGroup(job.Name)
			success := 0
			failed := 0
			for _, ex := range execs {
				if ex.Success {
					success = success + 1
				} else {
					failed = failed + 1
				}
			}

			if failed == 0 {
				return "success"
			} else if failed > 0 && success == 0 {
				return "danger"
			} else if failed > 0 && success > 0 {
				return "warning"
			}

			return ""
		},
		"jobJson": func(job *Job) string {
			j, _ := json.MarshalIndent(job, "", "<br>")
			return string(j)
		},
	}

	tmpl := template.Must(template.New("dashboard.html.tmpl").Funcs(funcs).ParseFiles(
		templateSet(a.config.UIDir, "jobs")...))

	data := struct {
		Common *commonDashboardData
		Jobs   []*Job
	}{
		Common: newCommonDashboardData(a, a.config.NodeName, "../"),
		Jobs:   jobs,
	}

	if err := tmpl.Execute(w, data); err != nil {
		log.Error(err)
	}
}

func (a *AgentCommand) dashboardExecutionsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")

	vars := mux.Vars(r)
	job := vars["job"]

	execs, _ := a.store.GetExecutions(job)
	groups := make(map[int64][]*Execution)
	for _, exec := range execs {
		groups[exec.Group] = append(groups[exec.Group], exec)
	}

	// Build a separate data structure to show in order
	var byGroup int64arr
	for key, _ := range groups {
		byGroup = append(byGroup, key)
	}
	sort.Sort(byGroup)

	tmpl := template.Must(template.New("dashboard.html.tmpl").Funcs(template.FuncMap{
		"html": func(value []byte) string {
			return string(template.HTML(value))
		},
		// Now unicode compliant
		"truncate": func(s string) string {
			var numRunes = 0
			for index, _ := range s {
				numRunes++
				if numRunes > 25 {
					return s[:index]
				}
			}
			return s
		},
	}).ParseFiles(templateSet(a.config.UIDir, "executions")...))

	if len(execs) > 100 {
		execs = execs[len(execs)-100:]
	}

	data := struct {
		Common  *commonDashboardData
		Groups  map[int64][]*Execution
		JobName string
		ByGroup int64arr
	}{
		Common:  newCommonDashboardData(a, a.config.NodeName, "../../../"),
		Groups:  groups,
		JobName: job,
		ByGroup: byGroup,
	}

	if err := tmpl.Execute(w, data); err != nil {
		log.Error(err)
	}
}
