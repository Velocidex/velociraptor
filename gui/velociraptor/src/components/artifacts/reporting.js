import "./reporting.css";
import _ from 'lodash';

import React from 'react';
import PropTypes from 'prop-types';
import axios from 'axios';
import parse from 'html-react-parser';

import api from '../core/api-service.js';
import VeloTable from '../core/table.js';
import { VeloLineChart, VeloTimeChart } from './line-charts.js';
import Spinner from '../utils/spinner.js';
import ToolViewer from "../tools/tool-viewer.js";
import Timeline from "../timeline/timeline.js";
import NotebookTableRenderer from '../notebooks/notebook-table-renderer.js';
import { NotebookLineChart, NotebookTimeChart,
         NotebookScatterChart, NotebookBarChart
       } from '../notebooks/notebook-chart-renderer.js';

// Renders a report in the DOM.

const parse_param = domNode=>JSON.parse(decodeURIComponent(
    domNode.attribs.params || "{}"));

// NOTE: The server sanitizes reports using the bluemonday
// sanitization. We therefore trust the output and are allowed to
// insert it into the DOM.

export default class VeloReportViewer extends React.Component {
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
        loading: true,
    }

    componentDidMount() {
        this.source = axios.CancelToken.source();
        this.updateReport();
    }

    componentWillUnmount() {
        this.source.cancel();
    }

    componentDidUpdate(prevProps) {
        let client_id = this.props.client && this.props.client.client_id;
        let artifact = this.props.artifact;

        let prev_client_id = prevProps.client && prevProps.client.client_id;

        if (client_id !== prev_client_id || artifact !== prevProps.artifact ||
            !_.isEqual(prevProps.params, this.props.params)) {
                this.updateReport();
        }
    }

    // Reports are generally pure.
    shouldComponentUpdate = (nextProps, nextState) => {
        let client_id = this.props.client && this.props.client.client_id;
        let next_client_id = nextProps.client && nextProps.client.client_id;

        return !_.isEqual(this.state, nextState) ||
            !_.isEqual(this.props.params, nextProps.params) ||
            !_.isEqual(client_id, next_client_id) ||
            !_.isEqual(this.props.artifact, nextProps.artifact);
    }

    // Notebook charts make API calls to actually get their data.
    maybeProcessNotebookChart = domNode=>{
        switch (domNode.name) {
        case "notebook-line-chart":
            return <NotebookLineChart params={parse_param(domNode)} />;

        case "notebook-bar-chart":
            return <NotebookBarChart params={parse_param(domNode)} />;

        case "notebook-scatter-chart":
            return <NotebookScatterChart params={parse_param(domNode)} />;

        case "notebook-time-chart":
            return <NotebookTimeChart params={parse_param(domNode)} />;

        default:
            return null;
        };
    };


    updateReport() {
        let client_id = this.props.client && this.props.client.client_id;
        let params = Object.assign({}, this.props.params || {});
        params.artifact = this.props.artifact;
        params.type = this.props.type;
        params.client_id = client_id;
        params.flow_id = this.props.flow_id;

        this.setState({loading: true});

        // Cancel any in flight calls.
        this.source.cancel();
        this.source = axios.CancelToken.source();

        api.post("v1/GetReport", params, this.source.token).then((response) => {
            if (response.cancel) return;

            let new_state  = {
                template: response.data.template || "No Reports",
                messages: response.data.messages || [],
                data: JSON.parse(response.data.data),
                loading: false,
            };

            for (var i=0; i<new_state.messages.length; i++) {
                console.log("While generating report: " + new_state.messages[i]);
            }
            this.setState(new_state);
        }).catch((err) => {
            let response = err.response && err.response.data;
            if (response) {
                let templ = "<div class='no-content'>" + response.message + "</div>";
                this.setState({"template": templ,loading: false});
            } else {
                this.setState({"template": "Error " + err.message, loading: false});
            }
        });
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
                if (domNode.name === "inline-table-viewer") {
                    try {
                        let data = this.state.data;
                        let value = decodeURIComponent(domNode.attribs.value || "");
                        let response = data[value] || {};
                        let rows = JSON.parse(response.Response);
                        return (
                            <VeloTable
                              rows={rows}
                              columns={response.Columns}
                            />
                        );
                    } catch(e) {

                    };
                }

                if (domNode.name === "grr-csv-viewer") {
                    // Figure out where the data is: attribs.value is something like data['table2']
                    let re = /'([^']+)'/;
                    let value = decodeURIComponent(domNode.attribs.value || "");
                    if (value) {
                        let match = re.exec(value);
                        let data = this.state.data[match[1]];
                        let rows = JSON.parse(data.Response);

                        return (
                            <VeloTable rows={rows} columns={data.Columns} />
                        );
                    }

                    try {
                        return (
                            <NotebookTableRenderer
                              refresh={this.props.refresh}
                              params={parse_param(domNode)}
                            />
                        );
                    } catch(e) {
                        return domNode;
                    }
                };

                if (domNode.name === "grr-tool-viewer") {
                    let name = decodeURIComponent(domNode.attribs.name ||"");
                    return (
                        <ToolViewer name={name}/>
                    );
                };

                if (domNode.name === "grr-timeline") {
                    let name = decodeURIComponent(domNode.attribs.name ||"");
                    return (
                        <Timeline name={name}/>
                    );
                };

                if (domNode.name === "grr-line-chart") {
                    // Figure out where the data is: attribs.value is
                    // something like data['table2']
                    let re = /'([^']+)'/;
                    let value = decodeURIComponent(domNode.attribs.value || "");
                    let match = re.exec(value);
                    let data = this.state.data[match[1]];
                    try {
                        let rows = JSON.parse(data.Response);
                        let params = JSON.parse(decodeURIComponent(domNode.attribs.params));
                        return (
                            <VeloLineChart data={rows}
                                           columns={data.Columns}
                                           params={params} />
                        );

                    } catch (e) {
                        return domNode;
                    }
                }

                if (domNode.name === "time-chart") {
                    // Figure out where the data is: attribs.value is
                    // something like data['table2']
                    let re = /'([^']+)'/;
                    let value = decodeURIComponent(domNode.attribs.value || "");
                    let match = re.exec(value);
                    let data = this.state.data[match[1]];
                    try {
                        let rows = JSON.parse(data.Response);
                        let params = JSON.parse(decodeURIComponent(domNode.attribs.params));
                        return (
                            <VeloTimeChart data={rows}
                                           columns={data.Columns}
                                           params={params} />
                        );

                    } catch (e) {
                        return domNode;
                    }
                };

                let res = this.maybeProcessNotebookChart(domNode);
                if (res) {
                    return res;
                }


                return domNode;
            }
        });

        return (
            <div className="report-viewer">
              <Spinner loading={this.state.loading} />
              {template}
            </div>
        );
    }
}
