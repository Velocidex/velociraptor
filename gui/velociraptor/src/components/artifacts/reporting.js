import "./reporting.css"

import React, { Component } from 'react';
import PropTypes from 'prop-types';

import parse from 'html-react-parser';

import api from '../core/api-service.js';
import VeloTable from '../core/table.js';

// Renders a report in the DOM.

// NOTE: The server sanitizes reports using the bluemonday
// sanitization. We therefore trust the output and are allowed to
// insert it into the DOM.

export default class VeloReportViewer extends Component {
    static propTypes = {
        artifact: PropTypes.string,
        type: PropTypes.string,
        client: PropTypes.object,
        flow_id: PropTypes.string,
    }

    state = {
        template: "",
        data: {},
        messages: [],
    }

    set = (k, v) => {
        let new_state  = Object.assign({}, this.state);
        new_state[k] = v;
        this.setState(new_state);
    }

    componentDidMount() {
        this.updateReport();
    }

    componentDidUpdate(prevProps) {
        if (!this.props.client || !this.props.client.client_id) {
            return;
        };

        let new_client_id = this.props.client.client_id;
        let prev_client_id = null;
        if (prevProps.client && prevProps.client.client_id) {
            prev_client_id = prevProps.client.client_id;
        }

        if (new_client_id != prev_client_id) {
            this.updateReport();
        }
    }

    updateReport() {
        if (!this.props.client || !this.props.client.client_id) {
            return;
        };

        let params = {
            artifact: this.props.artifact,
            type: this.props.type,
            client_id: this.props.client.client_id,
            flow_id: this.props.flow_id,
        };

        api.post("api/v1/GetReport", params).then(function(response) {
            let new_state  = {
                template: response.data.template || "No Reports",
                messages: response.data.messages || [],
                data: JSON.parse(response.data.data),
            };

            for (var i=0; i<new_state.messages.length; i++) {
                console.log("While generating report: " + new_state.messages[i]);
            }
            this.setState(new_state);
        }.bind(this), function(err) {
            this.set("template", "Error " + err.data.message);
        }.bind(this)).catch(function(err) {
            this.set("template", "Error " + err.message);
        }.bind(this));
    }

    render() {
        let template = parse(this.state.template, {
            replace: function(domNode) {
                if (domNode.name === "grr-csv-viewer") {
                    // Figure out where the data is: attribs.value is something like data['table2']
                    let re = /'([^']+)'/;
                    let match = re.exec(domNode.attribs.value);
                    let data = this.state.data[match[1]];
                    let rows = JSON.parse(data.Response);

                    return (
                        <VeloTable rows={rows} columns={data.Columns} />
                    );
                };
            }.bind(this),
        });

        return (
            <div className="report-viewer">{template}</div>
        );
    }
}
