import './file-list.css';
import _ from 'lodash';
import React, { Component } from 'react';
import PropTypes from 'prop-types';

import BootstrapTable from 'react-bootstrap-table-next';

import filterFactory from 'react-bootstrap-table2-filter';

import Navbar from 'react-bootstrap/Navbar';
import Button from 'react-bootstrap/Button';
import ButtonGroup from 'react-bootstrap/ButtonGroup';
import Modal from 'react-bootstrap/Modal';

import { Join } from '../utils/paths.js';
import api from '../core/api-service.js';
import axios from 'axios';

import { withRouter }  from "react-router-dom";

import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';

import { formatColumns } from "../core/table.js";

const POLL_TIME = 2000;

class DownloadAllDialog extends Component {
    static propTypes = {
        client: PropTypes.object,
        node: PropTypes.object,
        onCancel: PropTypes.func.isRequired,
        onClose: PropTypes.func.isRequired,
    }

    render() {
        let path = this.props.node.path.join("/");
        return (
            <Modal show={true} onHide={this.props.onCancel}>
              <Modal.Header closeButton>
                <Modal.Title>Recursively download files</Modal.Title>
              </Modal.Header>
              <Modal.Body>
                You are about to recursively fetch all files in <b>{path}</b>.
                <br/><br/>
                This may transfer a large amount of data from the endpoint. The default upload limit is 1gb but you can change it in the Collected Artifacts screen.
                </Modal.Body>
              <Modal.Footer>
                <Button variant="secondary" onClick={this.props.onCancel}>
                  Close
                </Button>
                <Button variant="primary" onClick={this.props.onClose}>
                  Yes do it!
                </Button>
              </Modal.Footer>
            </Modal>
        );
    }
}



class VeloFileList extends Component {
    static propTypes = {
        node: PropTypes.object,
        version: PropTypes.string,
        client: PropTypes.object,
        updateCurrentNode: PropTypes.func,
    }

    state = {
        // Manage single directory refresh.
        lastRefreshOperationId: null,

        // Manage recursive VFS listing
        lastRecursiveRefreshOperationId: null,
        lastRecursiveRefreshData: {},

        // Manage recursive VFS downloads
        showDownloadAllDialog: false,
        lastRecursiveDownloadOperationId: null,
        lastRecursiveDownloadData: {},
    }

    componentDidMount = () => {
        this.source = axios.CancelToken.source();
    }

    componentWillUnmount() {
        this.source.cancel("unmounted");
        if (this.interval) {
            clearInterval(this.interval);
        }
        if (this.recrusive_interval) {
            clearInterval(this.recrusive_interval);
        }
    }

    updateCurrentFile = (row) => {
        // We store the currently selected row in the node. When the
        // user updates the row, we change the node's internal
        // references.
        let node = this.props.node;
        if (!node) {
            return;
        }

        // Store the name of the selected row.
        node.selected = row.Name;
        this.props.updateCurrentNode(node);

        let path = this.props.node.path || [];
        // Update the router with the new path.
        let vfs_path = [...path];
        vfs_path.push(row.Name);
        this.props.history.push("/vfs/" + this.props.client.client_id +
                                Join(vfs_path));
    }

    startRecursiveVfsRefreshOperation = () => {
        // Only allow one refresh at the time.
        if (this.state.lastRecursiveRefreshOperationId) {
            return;
        }

        let path = this.props.node.path || [];
        api.post("v1/VFSRefreshDirectory", {
            client_id: this.props.client.client_id,
            vfs_path: Join(path),
            depth: 10
        }).then((response) => {
            // Hold onto the flow id.
            this.setState({lastRecursiveRefreshOperationId: response.data.flow_id});

            // Start polling for flow completion.
            this.recursive_interval = setInterval(() => {
                api.get("v1/GetFlowDetails", {
                    client_id: this.props.client.client_id,
                    flow_id: this.state.lastRecursiveRefreshOperationId,
                }).then((response) => {
                    let context = response.data.context;
                    if (context.state === "RUNNING") {
                        this.setState({lastRecursiveRefreshData: context});
                        return;
                    }

                    // The node is refreshed with the correct flow id, we can stop polling.
                    clearInterval(this.recursive_interval);
                    this.recursive_interval = undefined;

                    // Force a tree refresh since this flow is done.
                    let node = this.props.node;
                    node.version = this.state.lastRecursiveRefreshOperationId;
                    this.props.updateCurrentNode(this.props.node);
                    this.setState({
                        lastRecursiveRefreshData: {},
                        lastRecursiveRefreshOperationId: null});
                });
            }, POLL_TIME);

        });
    }

    cancelRecursiveRefresh = () => {
        api.post("v1/CancelFlow", {
            client_id: this.props.client.client_id,
            flow_id: this.state.lastRecursiveRefreshOperationId,
        });
    }

    startRecursiveVfsDownloadOperation = () => {
        // Only allow one refresh at the time.
        if (this.state.lastRecursiveDownloadOperationId) {
            return;
        }
        let node = this.props.node;

        // To recursively download files we launch an artifact
        // collection.
        let path = this.props.node.path || [];
        if (path.length < 1) {
            return;
        }

        let accessor = path[0];
        let search_path = Join(path.slice(1));
        api.post("v1/CollectArtifact", {
            client_id: this.props.client.client_id,
            artifacts: ["System.VFS.DownloadFile"],
            parameters: {"env": [
                { "key": "Path", "value": search_path},
                { "key": "Accessor", "value": accessor},
                { "key": "Recursively", "value": "Y"}]},
            max_upload_bytes: 1048576000,
        }).then((response) => {
            // Hold onto the flow id.
            this.setState({
                showDownloadAllDialog: false,
                lastRecursiveDownloadOperationId: response.data.flow_id});

            // Start polling for flow completion.
            this.recursive_download_interval = setInterval(() => {
                api.get("v1/GetFlowDetails", {
                    client_id: this.props.client.client_id,
                    flow_id: this.state.lastRecursiveDownloadOperationId,
                }).then((response) => {
                    let context = response.data.context;
                    if (context.state === "RUNNING") {
                        this.setState({lastRecursiveDownloadData: context});
                        return;
                    }

                    // The node is refreshed with the correct flow id, we can stop polling.
                    clearInterval(this.recursive_download_interval);
                    this.recursive_download_interval = undefined;

                    // Force a tree refresh since this flow is done.
                    node.version = this.state.lastRecursiveDownloadOperationId;
                    this.props.updateCurrentNode(this.props.node);
                    this.setState({
                        lastRecursiveDownloadData: {},
                        lastRecursiveDownloadOperationId: null});
                });
            }, POLL_TIME);

        });
    }

    cancelRecursiveDownload = () => {
        api.post("v1/CancelFlow", {
            client_id: this.props.client.client_id,
            flow_id: this.state.lastRecursiveDownloadOperationId,
        });
    }

    startVfsRefreshOperation = () => {
        // Only allow one refresh at the time.
        if (this.state.lastRefreshOperationId) {
            return;
        }

        let path = this.props.node.path || [];
        api.post("v1/VFSRefreshDirectory", {
            client_id: this.props.client.client_id,
            vfs_path: Join(path),
            depth: 0
        }).then((response) => {
            // Here we need to wait for the completion of the flow
            // *AND* the directory to be processed by the vfs
            // service. So it is not enough to just watch the flow
            // completion - we need to wait until the actual directory
            // is refreshed.
            this.setState({lastRefreshOperationId: response.data.flow_id});

            // Start polling for flow completion.
            this.source = axios.CancelToken.source();
            this.interval = setInterval(() => {
                api.get("v1/VFSStatDirectory", {
                    client_id: this.props.client.client_id,
                    vfs_path: Join(path),
                    flow_id: this.state.lastRefreshOperationId,
                }).then((response) => {
                    // The node is refreshed with the correct flow id, we can stop polling.
                    if (response.data.flow_id === this.state.lastRefreshOperationId) {
                        clearInterval(this.interval);
                        this.interval = undefined;

                        // Force a tree refresh since this flow is done.
                        let node = this.props.node;
                        node.version = this.state.lastRefreshOperationId;
                        this.props.updateCurrentNode(this.props.node);
                        this.setState({lastRefreshOperationId: null});
                    }
                });
            }, POLL_TIME);

        });
    }

    render() {
        // The node keeps track of which row is selected.
        let selected_name = this.props.node && this.props.node.selected;

        const selectRow = {
            mode: "radio",
            clickToSelect: true,
            hideSelectColumn: true,
            classes: "row-selected",
            onSelect: this.updateCurrentFile,
            selected: [selected_name],
        };

        let toolbar = (
            <Navbar className="toolbar">
              <ButtonGroup>
                { this.state.lastRefreshOperationId ?
                  <Button title="Currently refreshing"
                          onClick={this.startVfsRefreshOperation}
                          variant="default">
                    <FontAwesomeIcon icon="spinner" spin/>
                  </Button>
                  :
                  <Button title="Refresh this directory (sync its listing with the client)"
                          onClick={this.startVfsRefreshOperation}
                          variant="default">
                    <FontAwesomeIcon icon="folder-open"/>
                  </Button>
                }
                { this.state.lastRecursiveRefreshOperationId ?
                  <Button title="Currently refreshing from the client"
                          onClick={this.cancelRecursiveRefresh}
                          variant="default">
                    <FontAwesomeIcon icon="spinner" spin/>
                    <span className="button-label">Synced {(
                        this.state.lastRecursiveRefreshData.total_collected_rows || 0) + " files"}
                    </span>
                    <span className="button-label"><FontAwesomeIcon icon="stop"/></span>
                  </Button> :
                  <Button title="Recursively refresh this directory (sync its listing with the client)"
                          onClick={this.startRecursiveVfsRefreshOperation}
                          variant="default">
                    <FontAwesomeIcon icon="folder-open"/>
                    <span className="recursive-list-button">R</span>
                  </Button>
                }

                { this.state.lastRecursiveDownloadOperationId ?
                  <Button title="Currently fetching files from the client"
                          onClick={this.cancelRecursiveDownload}
                          variant="default">
                    <FontAwesomeIcon icon="spinner" spin/>
                    <span className="button-label">Downloaded {
                        (this.state.lastRecursiveDownloadData.total_uploaded_files || 0) + " files (" +
                            (this.state.lastRecursiveDownloadData.total_uploaded_bytes/1024/1024 ||
                             0).toFixed(0) + "Mb)"}
                    </span>
                    <span className="button-label"><FontAwesomeIcon icon="stop"/></span>
                  </Button> :
                  <Button title="Recursively download this directory from the client"
                          onClick={()=>this.setState({showDownloadAllDialog: true})}
                          disabled={_.isEmpty(this.props.node && this.props.node.path)}
                          variant="default">
                    <FontAwesomeIcon icon="file-download"/>
                    <span className="recursive-list-button">R</span>
                    &nbsp;
                  </Button>
                }

              </ButtonGroup>
            </Navbar>
        );

        if (!this.props.node || !this.props.node.raw_data) {
            return (
                <>
                  { toolbar }
                  <div className="fill-parent no-margins toolbar-margin">
                    <h5 className="no-content">No data available. Refresh directory from client by clicking above.</h5>
                  </div>
                </>
            );
        }

        let columns = formatColumns([
            {dataField: "Download", text: "", formatter: (cell, row) => {
                if (cell) {
                    return <FontAwesomeIcon icon="save"/>;
                }
                return <></>;
            }, sort: true},
            {dataField: "Name", text: "Name", sort: true, filtered: true},
            {dataField: "Size", text: "Size", sort: true},
            {dataField: "Mode", text: "Mode", sort: true},
            {dataField: "mtime", text: "mtime", sort: true},
            {dataField: "atime", text: "atime", sort: true},
            {dataField: "ctime", text: "ctime", sort: true}
        ]);

        return (
            <>
              { this.state.showDownloadAllDialog &&
                <DownloadAllDialog
                  node={this.props.node}
                  client={this.props.client}
                  onCancel={()=>this.setState({showDownloadAllDialog: false})}
                  onClose={this.startRecursiveVfsDownloadOperation}
                />}
              { toolbar }
              <div className="fill-parent no-margins toolbar-margin selectable">
                <BootstrapTable
                  hover
                  condensed
                  keyField="Name"
                  bootstrap4
                  headerClasses="alert alert-secondary"
                  bodyClasses="fixed-table-body"
                  data={this.props.node.raw_data}
                  columns={columns}
                  filter={ filterFactory() }
                  selectRow={ selectRow }
                />
              </div>
            </>
        );
    }
}

export default withRouter(VeloFileList);
