import './file-details.css';

import React from 'react';
import PropTypes from 'prop-types';
import T from '../i8n/i8n.jsx';

import VeloFileStats from './file-stats.jsx';
import FileHexView from './file-hex-view.jsx';
import FileTextView from './file-text-view.jsx';
import Tabs from 'react-bootstrap/Tabs';
import Tab from 'react-bootstrap/Tab';

export default class VeloFileDetails extends React.Component {
    static propTypes = {
        client: PropTypes.object,

        // The current node in the file tree.
        node: PropTypes.object,

        // The current selected row in the file list.
        selectedRow: PropTypes.object,
        version: PropTypes.string,
        updateCurrentNode: PropTypes.func,
    };

    state = {
        tab: "stats",
    }

    render() {
        let selectedRow = this.props.selectedRow;
        let has_download = selectedRow && selectedRow.Download && selectedRow.Download.mtime;

        return (
            <div className="padded">
              <Tabs defaultActiveKey="stats" onSelect={(tab) => this.setState({tab: tab})}>
                <Tab eventKey="stats" title="Stats">
                  <VeloFileStats
                    client={this.props.client}
                    node={this.props.node}
                    selectedRow={this.props.selectedRow}
                    updateCurrentNode={this.props.updateCurrentNode}
                  />
                </Tab>
                <Tab eventKey="text"
                     disabled={!has_download}
                     title={T("Textview")}>
                  { this.state.tab === "text" &&
                    <FileTextView
                      node={this.props.node}
                      selectedRow={this.props.selectedRow}
                      client={this.props.client}
                    />}
                </Tab>
                <Tab eventKey="hex"
                     disabled={!has_download}
                     title={T("HexView")}>
                  { this.state.tab === "hex" &&
                    <FileHexView
                      node={this.props.node}
                      selectedRow={this.props.selectedRow}
                      client={this.props.client}
                    />}
                </Tab>
              </Tabs>
            </div>
        );
    }
}
