import React from 'react';
import PropTypes from 'prop-types';

import parse from 'html-react-parser';
import VeloTable from '../core/table.js';

import NotebookTableRenderer from './notebook-table-renderer.js';

export default class NotebookReportRenderer extends React.Component {
    static propTypes = {
        refresh: PropTypes.func,
        cell: PropTypes.object,
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
                return domNode;
            }
        });
        return (
            <div  className="report-viewer">{template}</div>
        );
    }
};
