package web

import "fmt"

// connection.go builds the dashboard's graphical Connection info card from the
// Mythic .env config, mirroring exactly what the upstream PrintConnectionInfo()
// prints — the same hosts, ports, SSL and bind-localhost keys — but as a
// structured model we can render as cards/links instead of a stdout table.
//
// We read config through the envReader interface so the upstream coupling stays
// in adapter.go (which passes config.GetMythicEnv()) and the layout logic here
// is plain and testable.

// envReader is the slice of *viper.Viper we need to resolve connection details.
type envReader interface {
	GetString(key string) string
	GetInt(key string) int
	GetBool(key string) bool
}

// ConnectionEntry is one service's reachable address. The full URI is not shown
// in the UI — clicking the name copies Address to the clipboard; only the Port
// and bind scope are displayed.
//
// For services exposed on all interfaces, Address carries the hostPlaceholder
// token instead of a host: 127.0.0.1 wouldn't be reachable from a remote
// operator, so the browser substitutes its own location.hostname at copy time.
type ConnectionEntry struct {
	Name       string // "Nginx (Mythic Web UI)"
	Address    string // full access URI (may contain hostPlaceholder), copied on click
	Port       int    // bound port, shown next to the bind badge
	BoundLocal bool   // true = bound to 127.0.0.1 only, false = all interfaces
}

// hostPlaceholder marks where the browser's current hostname is substituted into
// a copied URI (see ConnectionEntry). Kept in sync with the replacement in the
// "scripts" template (base.html).
const hostPlaceholder = "{HOST}"

// ConnectionGroup is one labelled section of the card.
type ConnectionGroup struct {
	Title string
	Items []ConnectionEntry
}

// ConnectionModel is the whole Connection info card.
type ConnectionModel struct {
	Groups []ConnectionGroup
}

// connectionModel assembles the card from config, matching the two sections and
// ordering of the upstream PrintConnectionInfo() output.
func connectionModel(env envReader) ConnectionModel {
	return ConnectionModel{
		Groups: []ConnectionGroup{
			{
				Title: "Mythic services",
				Items: []ConnectionEntry{
					webEntry(env, "Nginx (Mythic Web UI)", "NGINX_HOST", "mythic_nginx", "NGINX_PORT", "nginx_bind_localhost_only", "NGINX_USE_SSL", ""),
					webEntry(env, "Mythic Backend Server", "MYTHIC_SERVER_HOST", "mythic_server", "MYTHIC_SERVER_PORT", "mythic_server_bind_localhost_only", "", ""),
					webEntry(env, "Hasura GraphQL Console", "HASURA_HOST", "mythic_graphql", "HASURA_PORT", "hasura_bind_localhost_only", "", ""),
					webEntry(env, "Jupyter Console", "JUPYTER_HOST", "mythic_jupyter", "JUPYTER_PORT", "jupyter_bind_localhost_only", "", ""),
					webEntry(env, "Internal Documentation", "DOCUMENTATION_HOST", "mythic_documentation", "DOCUMENTATION_PORT", "documentation_bind_localhost_only", "", ""),
				},
			},
			{
				Title: "Additional services",
				Items: []ConnectionEntry{
					postgresEntry(env),
					webEntry(env, "React Server", "MYTHIC_REACT_HOST", "mythic_react", "MYTHIC_REACT_PORT", "mythic_react_bind_localhost_only", "", "/new"),
					rabbitEntry(env),
				},
			},
		},
	}
}

// copyHost picks the host to put in a copied URI. A service bound to localhost
// only is reached at 127.0.0.1 (or its configured external host); an exposed
// service uses hostPlaceholder, swapped client-side for the browser's current
// hostname so the URI works from wherever the operator is.
func copyHost(env envReader, hostKey, container string, boundLocal bool) string {
	if !boundLocal {
		return hostPlaceholder
	}
	if h := env.GetString(hostKey); h != container {
		return h // explicit external host configured
	}
	return "127.0.0.1"
}

// webEntry builds an http(s) service entry. sslKey, when non-empty and true,
// switches the scheme to https; suffix appends a trailing path (e.g. "/new").
func webEntry(env envReader, name, hostKey, container, portKey, bindKey, sslKey, suffix string) ConnectionEntry {
	scheme := "http"
	if sslKey != "" && env.GetBool(sslKey) {
		scheme = "https"
	}
	port := env.GetInt(portKey)
	boundLocal := env.GetBool(bindKey)
	addr := fmt.Sprintf("%s://%s:%d%s", scheme, copyHost(env, hostKey, container, boundLocal), port, suffix)
	return ConnectionEntry{Name: name, Address: addr, Port: port, BoundLocal: boundLocal}
}

// postgresEntry and rabbitEntry are spelled out because their connection strings
// aren't plain http URLs.
func postgresEntry(env envReader) ConnectionEntry {
	port := env.GetInt("POSTGRES_PORT")
	boundLocal := env.GetBool("postgres_bind_localhost_only")
	addr := fmt.Sprintf("postgresql://mythic_user:password@%s:%d/mythic_db",
		copyHost(env, "POSTGRES_HOST", "mythic_postgres", boundLocal), port)
	return ConnectionEntry{Name: "Postgres Database", Address: addr, Port: port, BoundLocal: boundLocal}
}

func rabbitEntry(env envReader) ConnectionEntry {
	port := env.GetInt("RABBITMQ_PORT")
	boundLocal := env.GetBool("rabbitmq_bind_localhost_only")
	addr := fmt.Sprintf("amqp://%s:password@%s:%d",
		env.GetString("RABBITMQ_USER"), copyHost(env, "RABBITMQ_HOST", "mythic_rabbitmq", boundLocal), port)
	return ConnectionEntry{Name: "RabbitMQ", Address: addr, Port: port, BoundLocal: boundLocal}
}
