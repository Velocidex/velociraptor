import _ from 'lodash';

import React from 'react';
import PropTypes from 'prop-types';

import T from '../i8n/i8n.jsx';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';

import parseHTML from '../core/sanitize.jsx';

import TimelineRenderer from "../timeline/timeline.jsx";
import { VeloLineChart, VeloTimeChart } from '../artifacts/line-charts.jsx';
import { NotebookLineChart, NotebookTimeChart,
         NotebookScatterChart, NotebookBarChart
       } from './notebook-chart-renderer.jsx';

import ToolViewer from "../tools/tool-viewer.jsx";

import NotebookTableRenderer from './notebook-table-renderer.jsx';

import VeloValueRenderer from '../utils/value.jsx';
import { JSONparse } from '../utils/json_parse.jsx';

import VeloSigmaEditor from '../artifacts/sigma-editor.jsx';


const cleanupHTML = (html) => {
    // React expect no whitespace between table elements
    html = html.replace(/>\s*<thead/g, "><thead");
    html = html.replace(/>\s*<tbody/g, "><tbody");
    html = html.replace(/>\s*<tr/g, "><tr");
    html = html.replace(/>\s*<th/g, "><th");
    html = html.replace(/>\s*<td/g, "><td");

    html = html.replace(/>\s*<\/table/g, "></table");
    html = html.replace(/>\s*<\/thead/g, "></thead");
    html = html.replace(/>\s*<\/tbody/g, "></tbody");
    html = html.replace(/>\s*<\/tr/g, "></tr");
    html = html.replace(/>\s*<\/th/g, "></th");
    html = html.replace(/>\s*<\/td/g, "></td");
    return html;
};



const parse_param = domNode=>JSONparse(decodeURIComponent(
    domNode.attribs.params, {}));

export default class NotebookReportRenderer extends React.Component {
    static propTypes = {
        env: PropTypes.object,
        refresh: PropTypes.func,
        cell: PropTypes.object,
        notebook_id: PropTypes.string,

        // Will be called with addtional completions
        completion_reporter: PropTypes.func
,
    };

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

    // Inline charts contain their data in the HTML itself.
    maybeProcessInlineChart = domNode=>{
        if (_.isEmpty(domNode.attribs) ||
            _.isEmpty(domNode.attribs.value) ||
            _.isEmpty(domNode.attribs.params)) {
            return null;
        }

        // Figure out where the data is: attribs.value is
        // something like data['table2']
        let re = /'([^']+)'/;
        let value = decodeURIComponent(domNode.attribs.value || "");
        let match = re.exec(value);
        let data = this.state.data[match[1]];
        let rows = JSONparse(data.Response, []);

        switch  (domNode.name) {
        case "velo-line-chart":
                    return <VeloLineChart data={rows}
                                          columns={data.Columns}
                                          params={parse_param(domNode)} />;

        case  "time-chart":
                    return <VeloTimeChart data={rows}
                                          columns={data.Columns}
                                          params={parse_param(domNode)} />;

        default:
            return null;
        };
    }

    render() {
        let result = [];
        if (this.props.cell && this.props.cell.calculating && !this.props.cell.output) {
            result.push(<div className="padded" key="1">
                          {T("Calculating...")}
                          <span className="padded">
                            <FontAwesomeIcon icon="spinner" spin/>
                          </span>
                        </div>);
        }

        let output = this.props.cell && this.props.cell.output;
        if (!output) {
            result.push(<hr key="2"/>);
            return result;
        }

        let cell_id = this.props.cell && this.props.cell.cell_id;
        let template = parseHTML(cleanupHTML(this.props.cell.output), {
            replace: (domNode) => {
                if (domNode.name === "velo-value") {
                    let value = decodeURIComponent(domNode.attribs.value || "");
                    return <VeloValueRenderer value={value}/>;
                };

                if (domNode.name === "velo-timeline") {
                    let name = decodeURIComponent(domNode.attribs.name || "");
                    return (
                        <TimelineRenderer
                          notebook_id={this.props.notebook_id}
                          name={name}
                          params={parse_param(domNode)} />
                    );
                };

                if (domNode.name === "velo-sigma-editor") {
                    let params = JSONparse(decodeURIComponent(domNode.attribs.params), {});
                    return <VeloSigmaEditor
                             notebook_id={this.props.notebook_id}
                             cell={this.props.cell}
                             params={params}/>;
                }

                if (domNode.name ===  "velo-tool-viewer") {
                    let name = decodeURIComponent(domNode.attribs.name ||"");
                    let tool_version = decodeURIComponent(
                        domNode.attribs.version ||"");
                    return <ToolViewer name={name}
                                       tool_version={tool_version}/>;
                };

                // A tag that loads a table from a notebook cell.
                if (domNode.name === "velo-csv-viewer") {
                    try {
                        return (
                            <NotebookTableRenderer
                              env={this.props.env}
                              refresh={this.props.refresh}
                              params={parse_param(domNode)}
                              completion_reporter={this.props.completion_reporter}
                              name={cell_id}
                            />
                        );
                    } catch(e) {
                        return domNode;
                    }
                };

                let res = this.maybeProcessNotebookChart(domNode);
                if (res) {
                    return res;
                }

                res = this.maybeProcessInlineChart(domNode);
                if (res) {
                    return res;
                }

                return domNode;
            }
        });
        result.push(<div key="3" className="report-viewer">{template}</div>);
        return result;
    }
};
