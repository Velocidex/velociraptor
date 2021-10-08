import _ from 'lodash';

import React from 'react';
import PropTypes from 'prop-types';

import parse from 'html-react-parser';
import VeloTable from '../core/table.js';
import TimelineRenderer from "../timeline/timeline.js";
import { VeloLineChart, VeloTimeChart } from '../artifacts/line-charts.js';
import { NotebookLineChart, NotebookTimeChart,
         NotebookScatterChart, NotebookBarChart
       } from './notebook-chart-renderer.js';

import NotebookTableRenderer from './notebook-table-renderer.js';


const parse_param = domNode=>JSON.parse(decodeURIComponent(
    domNode.attribs.params || "{}"));

export default class NotebookReportRenderer extends React.Component {
    static propTypes = {
        refresh: PropTypes.func,
        cell: PropTypes.object,
        notebook_id: PropTypes.string,
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
        let output = this.props.cell && this.props.cell.output;
        if (!output) {
            return <hr/>;
        }

        let template = parse(this.props.cell.output, {
            replace: (domNode) => {
                // A table which contains the data inline.
                if (domNode.name === "inline-table-viewer") {
                    try {
                        let data = JSON.parse(this.props.cell.data || '{}');
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
                              refresh={this.props.refresh}
                              params={parse_param(domNode)}
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
        return (
            <div  className="report-viewer">{template}</div>
        );
    }
};
