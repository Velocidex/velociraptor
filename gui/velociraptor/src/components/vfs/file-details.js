import './file-details.css';

import React from 'react';
import PropTypes from 'prop-types';

import VeloFileStats from './file-stats.js';
import FileHexView from './file-hex-view.js';
import FileTextView from './file-text-view.js';

import Tabs from 'react-bootstrap/Tabs';
import Tab from 'react-bootstrap/Tab';

export default class VeloFileDetails extends React.Component {
    static propTypes = {
        client: PropTypes.object,
        node: PropTypes.object,
    };

    render() {
        let selectedRow = this.props.node && this.props.node.selected;

        if (!selectedRow || !selectedRow.Name) {
            return (
                <div className="card">
                  <h5 className="card-header">
                    Please select a file or a folder to see its details here.
                  </h5>
                </div>
            );
        }

        selectedRow = Object.assign({
            _FullPath: "",
            Name: "",
            mtime: "",
            atime: "",
            ctime: "",
            Mode: "",
            Size: 0,
            Download: {
                mtime: "",
                vfs_path: "",
                sparse: false,
            },
            _Data: {},
        }, selectedRow);

        return (
            <div className="padded">
              <grr-breadcrumbs path="controller.fileContext.selectedFilePath">
              </grr-breadcrumbs>

              <Tabs defaultActiveKey="stats">
                <Tab eventKey="stats" title="Stats">
                  <VeloFileStats
                    client={this.props.client}
                    selectedRow={selectedRow} />
                </Tab>
                <Tab eventKey="text" title="Textview">
                  <FileTextView
                    client={this.props.client}
                    selectedRow={selectedRow} />
                </Tab>
                <Tab eventKey="hex" title="HexView">
                  <FileHexView
                    client={this.props.client}
                    selectedRow={selectedRow} />
                </Tab>
                <Tab eventKey="table" title="TableView">
                  <VeloFileStats
                    client={this.props.client}
                    selectedRow={selectedRow} />
                </Tab>
                <Tab eventKey="reports" title="Reports">
                  <VeloFileStats
                    client={this.props.client}
                    selectedRow={selectedRow} />
                </Tab>
              </Tabs>
            </div>
        );
    }
};
