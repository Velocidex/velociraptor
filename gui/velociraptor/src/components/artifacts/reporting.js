import "./reporting.css";
import _ from 'lodash';

import React, { Component } from 'react';
import PropTypes from 'prop-types';

import parse from 'html-react-parser';

import api from '../core/api-service.js';
import VeloTable from '../core/table.js';
import VeloLineChart from './line-charts.js';

import ToolViewer from "../tools/tool-viewer.js";

// Renders a report in the DOM.

// NOTE: The server sanitizes reports using the bluemonday
// sanitization. We therefore trust the output and are allowed to
// insert it into the DOM.

export default class VeloReportViewer extends Component {
    static propTypes = {
        artifact: PropTypes.string,
        type: PropTypes.string,
        params: PropTypes.object,
        client: PropTypes.object,
        flow_id: PropTypes.string,
    }

    state = {
        template: "",
        data: {},
        messages: [],
    }

    componentDidMount() {
        this.updateReport();
    }

    componentDidUpdate(prevProps) {
        let client_id = this.props.client && this.props.client.client_id;
        let artifact = this.props.artifact;

        let prev_client_id = prevProps.client && prevProps.client.client_id;

        if (client_id !== prev_client_id || artifact !== prevProps.artifact) {
            this.updateReport();
        }

        if (!_.isEqual(prevProps.params, this.props.params)) {
            this.updateReport();
        }

    }

    updateReport() {
        let client_id = this.props.client && this.props.client.client_id;
        let params = this.props.params || {};
        params.artifact = this.props.artifact;
        params.type = this.props.type;
        params.client_id = client_id;
        params.flow_id = this.props.flow_id;

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
            this.setState({"template": "Error " + err.data.message});
        }.bind(this)).catch(function(err) {
            this.setState({"template": "Error " + err.message});
        }.bind(this));
    }

    cleanupHTML = (html) => {
        // React expect no whitespace between table elements
        html = html.replace(/>\s*<thead/g, "><thead");
        html = html.replace(/>\s*<tbody/g, "><tbody");
        html = html.replace(/>\s*<tr/g, "><tr");
        html = html.replace(/>\s*<th/g, "><th");
        html = html.replace(/>\s*<td/g, "><td");

        html = html.replace(/>\s*<\/thead/g, "></thead");
        html = html.replace(/>\s*<\/tbody/g, "></tbody");
        html = html.replace(/>\s*<\/tr/g, "></tr");
        html = html.replace(/>\s*<\/th/g, "></th");
        html = html.replace(/>\s*<\/td/g, "></td");
        return html;
    }

    render() {
        let template = parse(this.cleanupHTML(this.state.template), {
            replace: (domNode) => {
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
                if (domNode.name === "grr-tool-viewer") {
                    return (
                        <ToolViewer name={domNode.attribs.name}/>
                    );
                };
                if (domNode.name === "grr-line-chart") {
                    // Figure out where the data is: attribs.value is
                    // something like data['table2']
                    let re = /'([^']+)'/;
                    let match = re.exec(domNode.attribs.value);
                    let data = this.state.data[match[1]];
                    let rows = JSON.parse(data.Response);
                    let params = JSON.parse(domNode.attribs.params);

                    return (
                        <VeloLineChart data={rows}
                                       columns={data.Columns}
                                       params={params} />
                    );
                };

                return domNode;
            }
        });

        return (
            <div className="report-viewer">{template}</div>
        );
    }
}
