import React from 'react';
import PropTypes from 'prop-types';

import parse from 'html-react-parser';
import VeloTable from '../core/table.js';
import TimelineRenderer from "../timeline/timeline.js";
import VeloLineChart from '../artifacts/line-charts.js';

import NotebookTableRenderer from './notebook-table-renderer.js';

export default class NotebookReportRenderer extends React.Component {
    static propTypes = {
        refresh: PropTypes.func,
        cell: PropTypes.object,
        notebook_id: PropTypes.string,
    };

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
                        let response = data[domNode.attribs.value || "unknown"] || {};
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
                    return (
                        <TimelineRenderer
                          notebook_id={this.props.notebook_id}
                          name={domNode.attribs.name}
                          params={domNode.attribs.params}/>
                    );
                };

                if (domNode.name === "grr-csv-viewer") {
                    try {
                        let params = JSON.parse(domNode.attribs.params);
                        return (
                            <NotebookTableRenderer
                              refresh={this.props.refresh}
                              params={params}
                            />
                        );
                    } catch(e) {
                        return domNode;
                    }
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
            <div  className="report-viewer">{template}</div>
        );
    }
};
