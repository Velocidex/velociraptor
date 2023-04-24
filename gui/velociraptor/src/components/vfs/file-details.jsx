import './file-details.css';

import React from 'react';
import PropTypes from 'prop-types';
import T from '../i8n/i8n.jsx';

import VeloFileStats from './file-stats.jsx';

export default class VeloFileDetails extends React.PureComponent {
    static propTypes = {
        client: PropTypes.object,

        // The current node in the file tree.
        node: PropTypes.object,

        // The current selected row in the file list.
        selectedRow: PropTypes.object,
        version: PropTypes.string,
        updateCurrentNode: PropTypes.func,
    };

    render() {
        let selectedRow = this.props.selectedRow;
        let has_download = selectedRow &&
            selectedRow.Download && selectedRow.Download.mtime;

        return (
            <VeloFileStats
              client={this.props.client}
              node={this.props.node}
              selectedRow={this.props.selectedRow}
              updateCurrentNode={this.props.updateCurrentNode}
            />
        );
    }
}
