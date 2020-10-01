import React from 'react';
import PropTypes from 'prop-types';

import parse from 'html-react-parser';

import NotebookTableRenderer from './notebook-table-renderer.js';

export default class NotebookReportRenderer extends React.Component {
    static propTypes = {
        cell: PropTypes.object,
    };

    render() {
        let output = this.props.cell && this.props.cell.output;
        if (!output) {
            return <></>;
        }

        let template = parse(this.props.cell.output, {
            replace: (domNode) => {
                if (domNode.name === "grr-csv-viewer") {
                    let baseUrl = domNode.attribs["base-url"];
                    let params = JSON.parse(domNode.attribs.params);

                    return (
                        <NotebookTableRenderer
                          params={params}
                        />
                    );
                };
            }
        });
        return (
            <div  className="report-viewer">{template}</div>
        );
    }
};
