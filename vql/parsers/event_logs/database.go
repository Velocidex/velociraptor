package event_logs

import (
	"database/sql"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/evtx"
)

type DatabaseEnricher struct {
	db *sql.DB

	query *sql.Stmt
}

func (self *DatabaseEnricher) Enrich(event *ordereddict.Dict) *ordereddict.Dict {
	// Event.System.Provider.Name
	name, ok := ordereddict.GetString(event, "System.Provider.Name")
	if !ok {
		return event
	}

	event_id, ok := ordereddict.GetInt(event, "System.EventID.Value")
	if !ok {
		return event
	}

	rows, err := self.query.Query(name, event_id)
	if err != nil {
		return event
	}
	defer rows.Close()

	for rows.Next() {
		var message string
		err = rows.Scan(&message)
		if err == nil {
			event.Set("Message", evtx.ExpandMessage(event, message))
			break
		}
	}

	return event
}

func (self *DatabaseEnricher) Close() {
	self.db.Close()
}

func NewDatabaseEnricher(filename string) (*DatabaseEnricher, error) {
	result := &DatabaseEnricher{}

	if filename != "" {
		database, err := sql.Open("sqlite3", filename)
		if err != nil {
			return nil, err
		}

		result.db = database

		result.query, err = database.Prepare(`
          SELECT message
          FROM messages left join providers ON messages.provider_id = providers.id
          WHERE providers.name = ? and messages.event_id = ?
               `)
		if err != nil {
			database.Close()
			return nil, err
		}
	}

	return result, nil
}
