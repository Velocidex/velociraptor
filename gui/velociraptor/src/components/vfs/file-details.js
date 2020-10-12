import './file-details.css';

import React from 'react';
import PropTypes from 'prop-types';

import VeloFileStats from './file-stats.js';
import FileHexView from './file-hex-view.js';
import FileTextView from './file-text-view.js';
import utils from './utils.js';
import Tabs from 'react-bootstrap/Tabs';
import Tab from 'react-bootstrap/Tab';

export default class VeloFileDetails extends React.Component {
    static propTypes = {
        client: PropTypes.object,
        node: PropTypes.object,
        version: PropTypes.string,
        updateCurrentNode: PropTypes.func,
    };

    render() {
        let selected_name = this.props.node && this.props.node.selected;

        if (!selected_name) {
            return (
                <div className="card">
                  <h5 className="card-header">
                    Please select a file or a folder to see its details here.
                  </h5>
                </div>
            );
        }

        let selectedRow = utils.getSelectedRow(this.props.node);
        let has_download = selectedRow && selectedRow.Download && selectedRow.Download.mtime;

        return (
            <div className="padded">
              <grr-breadcrumbs path="controller.fileContext.selectedFilePath">
              </grr-breadcrumbs>

              <Tabs defaultActiveKey="stats">
                <Tab eventKey="stats" title="Stats">
                  <VeloFileStats
                    client={this.props.client}
                    node={this.props.node}
                    updateCurrentNode={this.props.updateCurrentNode}
                  />
                </Tab>
                <Tab eventKey="text"
                     disabled={!has_download}
                     title="Textview">
                  <FileTextView
                    client={this.props.client}
                    node={this.props.node} />
                </Tab>
                <Tab eventKey="hex"
                     disabled={!has_download}
                     title="HexView">
                  <FileHexView
                    node={this.props.node}
                    client={this.props.client}
                  />
                </Tab>
              </Tabs>
            </div>
        );
    }
};
