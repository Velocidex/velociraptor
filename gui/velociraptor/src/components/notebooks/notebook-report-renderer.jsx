import _ from 'lodash';

import React from 'react';
import PropTypes from 'prop-types';

import T from '../i8n/i8n.jsx';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';

import Button from 'react-bootstrap/Button';
import parseHTML from '../core/sanitize.jsx';

import VeloTable from '../core/table.jsx';
import TimelineRenderer from "../timeline/timeline.jsx";
import { VeloLineChart, VeloTimeChart } from '../artifacts/line-charts.jsx';
import { NotebookLineChart, NotebookTimeChart,
         NotebookScatterChart, NotebookBarChart
       } from './notebook-chart-renderer.jsx';

import NotebookTableRenderer from './notebook-table-renderer.jsx';

import VeloValueRenderer from '../utils/value.jsx';

const parse_param = domNode=>JSON.parse(decodeURIComponent(
    domNode.attribs.params || "{}"));

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
        let rows = JSON.parse(data.Response);

        switch  (domNode.name) {
        case "grr-line-chart":
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

        let template = parseHTML(this.props.cell.output, {
            replace: (domNode) => {
                // A table which contains the data inline.
                if (domNode.name === "inline-table-viewer") {
                    try {
                        let data = JSON.parse(this.props.cell.data || '{}');
                        let value = decodeURIComponent(domNode.attribs.value || "");
                        let response = data[value] || {};
                        let rows = JSON.parse(response.Response);
                        if(this.props.completion_reporter) {
                            this.props.completion_reporter(response.Columns);
                        }

                        return (
                            <VeloTable
                              env={this.props.env}
                              rows={rows}
                              columns={response.Columns}
                            />
                        );
                    } catch(e) {

                    };
                }

                if (domNode.name === "velo-value") {
                    let value = decodeURIComponent(domNode.attribs.value || "");
                    return <VeloValueRenderer value={value}/>;
                };

                if (domNode.name === "grr-timeline") {
                    let name = decodeURIComponent(domNode.attribs.name || "");
                    return (
                        <TimelineRenderer
                          notebook_id={this.props.notebook_id}
                          name={name}
                          params={parse_param(domNode)} />
                    );
                };

                // A tag that loads a table from a notebook cell.
                if (domNode.name === "grr-csv-viewer") {
                    try {
                        return (
                            <NotebookTableRenderer
                              env={this.props.env}
                              refresh={this.props.refresh}
                              params={parse_param(domNode)}
                              completion_reporter={this.props.completion_reporter}
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
