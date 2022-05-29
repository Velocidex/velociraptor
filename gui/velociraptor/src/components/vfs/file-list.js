import './file-list.css';
import _ from 'lodash';
import React, { Component } from 'react';
import PropTypes from 'prop-types';
import classNames from "classnames";
import BootstrapTable from 'react-bootstrap-table-next';

import filterFactory from 'react-bootstrap-table2-filter';
import { Link } from  "react-router-dom";

import Navbar from 'react-bootstrap/Navbar';
import Button from 'react-bootstrap/Button';
import ButtonGroup from 'react-bootstrap/ButtonGroup';
import Modal from 'react-bootstrap/Modal';

import { Join, EncodePathInURL } from '../utils/paths.js';
import api from '../core/api-service.js';
import axios from 'axios';

import { withRouter }  from "react-router-dom";

import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';

import T from '../i8n/i8n.js';
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
                <Modal.Title>{T("Recursively download files")}</Modal.Title>
              </Modal.Header>
              <Modal.Body>
                {T("RecursiveVFSMessgae")}
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
        this.props.history.push(
           EncodePathInURL("/vfs/"+ this.props.client.client_id + Join(vfs_path)));
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
        let search_path = full_path;
        api.post("v1/CollectArtifact", {
            urgent: true,
            client_id: this.props.client.client_id,
            artifacts: ["System.VFS.DownloadFile"],
            specs: [{artifact: "System.VFS.DownloadFile",
                     parameters: {"env": [
                         { "key": "Path", "value": search_path},
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
                  <Button title={T("Currently refreshing")}
                          onClick={this.startVfsRefreshOperation}
                          variant="default">
                    <FontAwesomeIcon icon="spinner" spin/>
                  </Button>
                  :
                  <Button title={T("Refresh this directory (sync its listing with the client)")}
                          onClick={this.startVfsRefreshOperation}
                          variant="default">
                    <FontAwesomeIcon icon="folder-open"/>
                  </Button>
                }
                { this.state.lastRecursiveRefreshOperationId ?
                  <Button title={T("Currently refreshing from the client")}
                          onClick={this.cancelRecursiveRefresh}
                          variant="default">
                    <FontAwesomeIcon icon="spinner" spin/>
                    <span className="button-label">Synced {(
                        this.state.lastRecursiveRefreshData.total_collected_rows || 0) + " files"}
                    </span>
                    <span className="button-label"><FontAwesomeIcon icon="stop"/></span>
                  </Button> :
                  <Button title={T("Recursively refresh this directory (sync its listing with the client)")}
                          onClick={this.startRecursiveVfsRefreshOperation}
                          variant="default">
                    <FontAwesomeIcon icon="folder-open"/>
                    <span className="recursive-list-button">R</span>
                  </Button>
                }

                { this.state.lastRecursiveDownloadOperationId ?
                  <Button title={T("Currently fetching files from the client")}
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
                  <Button title={T("Recursively download this directory from the client")}
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
                      title={T("View Collection")}
                      role="button"
                      className={classNames({
                          "btn": true,
                          "btn-default": true,
                          "disabled":  !this.props.node.flow_id,
                      })}>
                  <FontAwesomeIcon icon="eye"/>
                </Link>
              </ButtonGroup>
            </Navbar>
        );

        if (!this.props.node || !this.props.node.raw_data) {
            return (
                <>
                  { toolbar }
                  <div className="fill-parent no-margins toolbar-margin">
                    <h5 className="no-content">{T("No data available. Refresh directory from client by clicking above.")}</h5>
                  </div>
                </>
            );
        }

        if (_.isEmpty(this.props.node.raw_data)) {
            return <>
                     { toolbar }
                     <div className="fill-parent no-margins toolbar-margin">
                       <h5 className="no-content">Directory is empty.</h5>
                     </div>
                   </>;
        }

        let columns = formatColumns([
            {dataField: "Download", text: "", formatter: (cell, row) => {
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
            }, sort: true, classes: "download-column"},
            {dataField: "Name", text: T("Name"), sort: true, filtered: true},
            {dataField: "Size", text: T("Size"), sort: true, type: "mb"},
            {dataField: "Mode", text: T("Mode"), sort: true},
            {dataField: "mtime", text: T("mtime"), sort: true, type: "timestamp"},
            {dataField: "atime", text: T("atime"), sort: true, type: "timestamp"},
            {dataField: "ctime", text: T("ctime"), sort: true, type: "timestamp"},
            {dataField: "btime", text: T("btime"), sort: true, type: "timestamp"}
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
