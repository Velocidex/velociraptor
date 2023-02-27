import './file-list.css';
import _ from 'lodash';
import React, { Component } from 'react';
import PropTypes from 'prop-types';
import classNames from "classnames";

import { Link, withRouter } from  "react-router-dom";

import Navbar from 'react-bootstrap/Navbar';
import Button from 'react-bootstrap/Button';
import ButtonGroup from 'react-bootstrap/ButtonGroup';
import Modal from 'react-bootstrap/Modal';

import VeloPagedTable from '../core/paged-table.jsx';

import { Join, EncodePathInURL } from '../utils/paths.jsx';
import api from '../core/api-service.jsx';
import axios from 'axios';

import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';

import T from '../i8n/i8n.jsx';

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
                <Modal.Title>{T("Recursively download files")}</Modal.Title>
              </Modal.Header>
              <Modal.Body>
                {T("RecursiveVFSMessage", path)}
                </Modal.Body>
              <Modal.Footer>
                <Button variant="secondary" onClick={this.props.onCancel}>
                  {T("Close")}
                </Button>
                <Button variant="primary" onClick={this.props.onClose}>
                  {T("Yes do it!")}
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
        updateCurrentSelectedRow: PropTypes.func,
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
        selectedFile: "",
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
        this.setState({ selectedFile: row._id});
        this.props.updateCurrentSelectedRow(row);
        this.props.updateCurrentNode(this.props.node);

        let path = this.props.node.path || [];
        // Update the router with the new path.
        let vfs_path = [...path];
        vfs_path.push(row.Name);
        this.props.history.push(
           "/vfs/"+ this.props.client.client_id + EncodePathInURL(Join(vfs_path)));
    }

    startRecursiveVfsRefreshOperation = () => {
        // Only allow one refresh at the time.
        if (this.state.lastRecursiveRefreshOperationId) {
            return;
        }

        let path = this.props.node.path || [];
        api.post("v1/VFSRefreshDirectory", {
            client_id: this.props.client.client_id,
            vfs_components: path,
            depth: 10
        }, this.source.token).then((response) => {
            // Hold onto the flow id.
            this.setState({lastRecursiveRefreshOperationId: response.data.flow_id});

            // Start polling for flow completion.
            this.recursive_interval = setInterval(() => {
                api.get("v1/GetFlowDetails", {
                    client_id: this.props.client.client_id,
                    flow_id: this.state.lastRecursiveRefreshOperationId,
                }, this.source.token).then((response) => {
                    if (!response.data || !response.data.context) {
                        return;
                    }
                    let context = response.data.context;
                    if (context.state === "RUNNING") {
                        this.setState({lastRecursiveRefreshData: context});
                        return;
                    }

                    // The node is refreshed with the correct flow id,
                    // we can stop polling.
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
        }, this.source.token);
    }

    startRecursiveVfsDownloadOperation = () => {
        // Only allow one refresh at the time.
        if (this.state.lastRecursiveDownloadOperationId) {
            return;
        }
        let node = this.props.node;

        // To recursively download files we launch an artifact
        // collection.

        //  full_path is the client side path as told by the client's
        //  VFSListDirectory.
        let full_path = this.props.node && this.props.node.full_path;
        if (!full_path || !node.path) {
            return;
        }

        // path is a list of components starting with the accessor
        let accessor = node.path[0];
        api.post("v1/CollectArtifact", {
            urgent: true,
            client_id: this.props.client.client_id,
            artifacts: ["System.VFS.DownloadFile"],
            specs: [{artifact: "System.VFS.DownloadFile",
                     parameters: {"env": [
                         { "key": "Components",
                           "value": JSON.stringify(node.path.slice(1))},
                         { "key": "Accessor", "value": accessor},
                         { "key": "Recursively", "value": "Y"}]}}],
            max_upload_bytes: 1048576000,
        }, this.source.token).then((response) => {
            // Hold onto the flow id.
            this.setState({
                showDownloadAllDialog: false,
                lastRecursiveDownloadOperationId: response.data.flow_id});

            // Start polling for flow completion.
            this.recursive_download_interval = setInterval(() => {
                api.get("v1/GetFlowDetails", {
                    client_id: this.props.client.client_id,
                    flow_id: this.state.lastRecursiveDownloadOperationId,
                }, this.source.token).then((response) => {
                    let context = response.data.context;
                    if (!context || context.state === "RUNNING") {
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
        }, this.source.token);
    }

    startVfsRefreshOperation = () => {
        // Only allow one refresh at the time.
        if (this.state.lastRefreshOperationId) {
            return;
        }

        let path = this.props.node.path || [];
        api.post("v1/VFSRefreshDirectory", {
            client_id: this.props.client.client_id,
            vfs_components: path,
            depth: 0
        }, this.source.token).then((response) => {
            // Here we need to wait for the completion of the flow
            // *AND* the directory to be processed by the vfs
            // service. So it is not enough to just watch the flow
            // completion - we need to wait until the actual directory
            // is refreshed.
            this.setState({lastRefreshOperationId: response.data.flow_id});

            // Start polling for flow completion.
            this.source = axios.CancelToken.source();
            this.interval = setInterval(() => {
                // If it not enough here to just wait for the flow to
                // finish, we need to wait for the vfs service to
                // actually write the new directory entry otherwise
                // the gui can refresh with the old data still there.
                api.get("v1/VFSStatDirectory", {
                    client_id: this.props.client.client_id,
                    vfs_components: path,
                    flow_id: this.state.lastRefreshOperationId,
                }, this.source.token).then((response) => {
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
        const selectRow = {
            mode: "radio",
            clickToSelect: true,
            hideSelectColumn: true,
            classes: "row-selected",
            onSelect: this.updateCurrentFile,
            selected: [this.state.selectedFile],
        };

        let toolbar = (
              <ButtonGroup className="float-right vfs-toolbar">
                { this.state.lastRefreshOperationId ?
                  <Button data-tooltip={T("Currently refreshing")}
                          data-position="right"
                          className="btn-tooltip"
                          onClick={this.startVfsRefreshOperation}
                          variant="default">
                    <FontAwesomeIcon icon="spinner" spin/>
                  </Button>
                  :
                  <Button data-tooltip={T("Refresh this directory (sync its listing with the client)")}
                          data-position="right"
                          disabled={!this.props.node.path}
                          className="btn-tooltip"
                          onClick={this.startVfsRefreshOperation}
                          variant="default">
                    <FontAwesomeIcon icon="folder-open"/>
                  </Button>
                }
                { this.state.lastRecursiveRefreshOperationId ?
                  <Button data-tooltip={T("Currently refreshing from the client")}
                          data-position="right"
                          className="btn-tooltip"
                          onClick={this.cancelRecursiveRefresh}
                          variant="default">
                    <FontAwesomeIcon icon="spinner" spin/>
                    <span className="button-label">Synced {(
                        this.state.lastRecursiveRefreshData.total_collected_rows || 0) + " files"}
                    </span>
                    <span className="button-label"><FontAwesomeIcon icon="stop"/></span>
                  </Button> :
                  <Button data-tooltip={T("Recursively refresh this directory (sync its listing with the client)")}
                          data-position="right"
                          className="btn-tooltip"
                          disabled={!this.props.node.path}
                          onClick={this.startRecursiveVfsRefreshOperation}
                          variant="default">
                    <FontAwesomeIcon icon="folder-open"/>
                    <span className="recursive-list-button">R</span>
                  </Button>
                }

                { this.state.lastRecursiveDownloadOperationId ?
                  <Button data-tooltip={T("Currently fetching files from the client")}
                          data-position="right"
                          className="btn-tooltip"
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
                  <Button data-tooltip={T("Recursively download this directory from the client")}
                          data-position="right"
                          className="btn-tooltip"
                          onClick={()=>this.setState({showDownloadAllDialog: true})}
                          disabled={_.isEmpty(this.props.node && this.props.node.path)}
                          variant="default">
                    <FontAwesomeIcon icon="file-download"/>
                    <span className="recursive-list-button">R</span>
                    &nbsp;
                  </Button>
                }

                <Link to={"/collected/" +this.props.client.client_id +
                          "/" + this.props.node.flow_id + "/overview"}
                      data-tooltip={T("View Collection")}
                      data-position="right"
                      role="button"
                      className={classNames({
                          "btn": true,
                          "btn-tooltip": true,
                          "btn-default": true,
                          "disabled":  !this.props.node.flow_id,
                      })}>
                  <FontAwesomeIcon icon="eye"/>
                </Link>
              </ButtonGroup>
        );

        if (!this.props.node || !this.props.node.flow_id) {
            return (
              <div className="fill-parent no-margins selectable col-12">
                <Navbar className="toolbar">
                  { toolbar }
                </Navbar>
                <div className="fill-parent no-margins toolbar-margin">
                  <h5 className="no-content">
                    {T("No data available. Refresh directory from client by clicking above.")}
                  </h5>
                </div>
              </div>
            );
        }

        // directory is empty
        if (this.props.node.start_idx === this.props.node.end_idx) {
            return (
              <div className="fill-parent no-margins selectable col-12">
                <Navbar className="toolbar">
                  { toolbar }
                </Navbar>
                <div className="fill-parent no-margins toolbar-margin">
                  <h5 className="no-content">
                    {T("Directory is empty.")}
                  </h5>
                </div>
              </div>
            );
        }

        let formatters = {
            "Download": (cell, row) => {
                let result = [];
                if (cell) {
                    result.push( <FontAwesomeIcon
                                   className="hint-icon"
                                   key="1" icon="save"/>);
                }
                let fn_btime = row["_Data"] && row._Data["fn_btime"];
                let btime = row["btime"];
                if (btime < fn_btime) {
                    result.push( <FontAwesomeIcon
                                   key="2"
                                   className="file-stomped hint-icon"
                                   icon="clock"/>);
                }
                //return <FontAwesomeIcon icon="clock"/>;
                return <span className="file-hints">{result}</span>;
            },
            "Size": (cell, row) => {
                let result = cell/1024/1024;
                let value = cell;
                let suffix = "";
                if (_.isFinite(result) && result > 0) {
                    suffix = "Mb";
                    value = result.toFixed(0);
                } else {
                    result = (cell /1024).toFixed(0);
                    if (_.isFinite(result) && result > 0) {
                        suffix = "Kb";
                        value = result.toFixed(0);
                    } else {
                        if (_.isFinite(cell)) {
                            suffix = "b";
                            value = cell.toFixed(0);
                        }
                    }
                }

                return value + " " + suffix;
            },
        };

        return (
            <>
              { this.state.showDownloadAllDialog &&
                <DownloadAllDialog
                  node={this.props.node}
                  client={this.props.client}
                  onCancel={()=>this.setState({showDownloadAllDialog: false})}
                  onClose={this.startRecursiveVfsDownloadOperation}
                />}
              <div className="fill-parent no-margins selectable">
                <VeloPagedTable
                  url="v1/VFSListDirectoryFiles"
                  renderers={formatters}
                  version={{
                      version: this.props.node && this.props.node.version,
                  }}
                  params={{
                      client_id: this.props.client.client_id,
                      flow_id: this.props.node.flow_id,
                      vfs_components: this.props.node.path,
                  }}
                  selectRow={ selectRow }
                  toolbar={toolbar}
                  initial_page_size={5}
                />
              </div>
            </>
        );
    }
}

export default withRouter(VeloFileList);
