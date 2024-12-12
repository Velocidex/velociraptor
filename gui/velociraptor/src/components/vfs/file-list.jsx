import './file-list.css';
import _ from 'lodash';
import React, { Component } from 'react';
import PropTypes from 'prop-types';
import classNames from "classnames";

import { Link, withRouter } from  "react-router-dom";
import ToolTip from '../widgets/tooltip.jsx';

import Navbar from 'react-bootstrap/Navbar';
import Button from 'react-bootstrap/Button';
import ButtonGroup from 'react-bootstrap/ButtonGroup';
import Modal from 'react-bootstrap/Modal';
import VeloTimestamp from "../utils/time.jsx";
import VeloPagedTable from '../core/paged-table.jsx';

import { Join, EncodePathInURL } from '../utils/paths.jsx';
import api from '../core/api-service.jsx';
import {CancelToken} from 'axios';

import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import VeloForm from "../forms/form.jsx";
import FlowLink from "../flows/flow-link.jsx";

import T from '../i8n/i8n.jsx';

import {
  Menu,
  Item,
  useContextMenu,
} from "react-contexify";

// If the flow in the running state. Flows can be in several states
// that mean they are still running for our purposes.
const is_running_re = new RegExp("running|waiting|in_progress", "i");
const is_running = x=>is_running_re.test(x);

const POLL_TIME = 2000;

class DownloadAllDialog extends Component {
    static propTypes = {
        client: PropTypes.object,
        node: PropTypes.object,
        onCancel: PropTypes.func.isRequired,
        onClose: PropTypes.func.isRequired,

        // React router props.
        match: PropTypes.object,
        history: PropTypes.object,
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

class ExportDialog extends Component {
    static propTypes = {
        client: PropTypes.object,
        node: PropTypes.object,
        onCancel: PropTypes.func.isRequired,
        onClose: PropTypes.func.isRequired,
    }

    state = {
        glob: "/**",
        flow_id: "",
    }

    componentDidMount = () => {
        this.source = CancelToken.source();
    }

    componentWillUnmount() {
        this.source.cancel();
    }

    doIt = ()=>{
        let client_id = this.props.client && this.props.client.client_id;
        api.post("v1/CollectArtifact", {
            client_id: "server",
            artifacts: ["System.VFS.Export"],
            specs: [{artifact: "System.VFS.Export",
                     parameters: {env: [
                         {key: "Components",
                          value: JSON.stringify(this.props.node.path)},
                         {key: "ClientId", value: client_id},
                         {key: "FileGlob", value: this.state.glob || "/**"},
                     ]}}],
            max_upload_bytes: 1000000000000,
        }, this.source.token).then((resp)=>{
            if(resp.data) {
                this.setState({flow_id: resp.data.flow_id});
            };
        });
    }

    render() {
        let path = "/" + this.props.node.path.join("/");
        return (
            <Modal show={true} onHide={this.props.onCancel}>
              <Modal.Header closeButton>
                <Modal.Title>{T("Export VFS Files")}</Modal.Title>
              </Modal.Header>
              <Modal.Body>
                {T("Collect files from the VFS starting from ")}
                <div className="vfspath">{path}</div>
                { this.state.flow_id ?
                  <FlowLink
                    flow_id={this.state.flow_id}
                    client_id="server"
                  />
                  :
                  <VeloForm
                    value={this.state.glob}
                    setValue={x=>this.setState({glob: x})}
                    param={{name: T("Glob"), description: T("Export file glob")}}
                  />
                }
              </Modal.Body>
              <Modal.Footer>
                <Button variant="secondary" onClick={this.props.onCancel}>
                  {T("Close")}
                </Button>
                { !this.state.flow_id &&
                  <Button variant="primary" onClick={this.doIt}>
                    {T("Yes do it!")}
                  </Button>
                }
              </Modal.Footer>
            </Modal>
        );
    }
}


function DownloadContextMenu({children, value}) {
    const { show } = useContextMenu({
        id: "download-menu-id",
        props: {
            value: value,
        }
    });
    return (
        <div onContextMenu={show}>{children}</div>
    );
}

DownloadContextMenu.propTypes = {
    children: PropTypes.node,
    value: PropTypes.any,
};

class VeloFileList extends Component {
    static propTypes = {
        node: PropTypes.object,
        version: PropTypes.string,
        client: PropTypes.object,
        updateCurrentSelectedRow: PropTypes.func,
        updateCurrentNode: PropTypes.func,
        collapseToggle: PropTypes.func,
        bumpVersion: PropTypes.func,

        // React router props.
        history: PropTypes.object,
    }

    state = {
        // Manage recursive VFS listing
        lastRecursiveRefreshFlowId: null,
        lastRecursiveRefreshData: {},

        // Manage recursive VFS downloads
        showDownloadAllDialog: false,
        showExportDialog: false,
        lastRecursiveDownloadFlowId: null,
        lastRecursiveDownloadData: {},
        selectedFile: "",

        selectedTableIdx: 0,
    }

    componentDidMount = () => {
        this.source = CancelToken.source();
        this.interval = setInterval(this.statDirectory, POLL_TIME);
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
        // references. (We need to pass the currently selected row to
        // the parent component so the bottom pane can be updated.)
        let node = this.props.node;
        if (!node) {
            return;
        }

        // Store the name of the selected row.
        this.setState({ selectedFile: row._id, selectedTableIdx: row._Idx});
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
        this.setState({current_path: this.props.node.path,
                       recursive_sync_dir_version: this.props.version});

        let path = this.props.node.path || [];
        api.post("v1/VFSRefreshDirectory", {
            client_id: this.props.client.client_id,
            vfs_components: path,
            depth: 10
        }, this.source.token).then((response) => {
            // If there is an error with calling the API cancel the spin immediately
            if (!response.data || response.data.cancel ||
                !response.data.flow_id) {
                this.setState({recrusive_sync_dir_version: this.props.version});
                return;
            }

            // Hold onto the flow id so we can track progress.
            this.setState({lastRecursiveRefreshFlowId: response.data.flow_id});

            if (this.recursive_interval) {
                clearInterval(this.recursive_interval);
            }

            // Start polling for flow completion.
            this.recursive_interval = setInterval(() => {
                api.get("v1/GetFlowDetails", {
                    client_id: this.props.client.client_id,
                    flow_id: this.state.lastRecursiveRefreshFlowId,
                }, this.source.token).then((response) => {
                    if (!response.data || !response.data.context) {
                        clearInterval(this.recursive_interval);
                        this.recursive_interval = undefined;
                        return;
                    }
                    let context = response.data.context;
                    if (is_running(context.state)) {
                        this.setState({lastRecursiveRefreshData: context});
                        return;
                    }

                    // The node is refreshed with the correct flow id,
                    // we can stop polling.
                    clearInterval(this.recursive_interval);
                    this.recursive_interval = undefined;
                    this.props.bumpVersion();
                    this.setState({
                        lastRecursiveRefreshData: {},
                        lastRecursiveRefreshFlowId: null});
                });
            }, POLL_TIME);

        });
    }

    cancelRecursiveRefresh = () => {
        api.post("v1/CancelFlow", {
            client_id: this.props.client.client_id,
            flow_id: this.state.lastRecursiveRefreshFlowId,
        }, this.source.token).then(response=>{
            this.setState({recrusive_sync_dir_version: this.props.version});
        });
    }

    startRecursiveVfsDownloadOperation = () => {
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
            // Hold onto the flow id so we can track progress.
            this.setState({
                showDownloadAllDialog: false,
                lastRecursiveDownloadFlowId: response.data.flow_id});

            // If there is already an operation in progress then just
            // cancel it.
            if (this.recursive_download_interval) {
                clearInterval(this.recursive_download_interval);
            }

            // Start polling for flow completion.
            this.recursive_download_interval = setInterval(() => {
                api.get("v1/GetFlowDetails", {
                    client_id: this.props.client.client_id,
                    flow_id: this.state.lastRecursiveDownloadFlowId,
                }, this.source.token).then((response) => {
                    let context = response.data && response.data.context;
                    if (!context || is_running(context.state)) {
                        this.setState({lastRecursiveDownloadData: context});
                        return;
                    }

                    // The node is refreshed with the correct flow id,
                    // we can stop polling.
                    clearInterval(this.recursive_download_interval);
                    this.recursive_download_interval = undefined;

                    // Force a tree refresh since this flow is done.
                    this.setState({
                        lastRecursiveDownloadData: {},
                        lastRecursiveDownloadFlowId: null});
                });
            }, POLL_TIME);

        });
    }

    cancelRecursiveDownload = () => {
        api.post("v1/CancelFlow", {
            client_id: this.props.client.client_id,
            flow_id: this.state.lastRecursiveDownloadFlowId,
        }, this.source.token);
    }

    // Determine if the SyncDir button should be spinning.
    shouldSpinSyncDir = ()=>{
        return _.isEqual(this.state.current_path, this.props.node.path) &&
            _.isEqual(this.state.sync_dir_version, this.props.version);
    }

    // Determine if the recursive SyncDir button should spin.
    shouldSpinRecursiveSyncDir = ()=>{
        if (_.isEmpty(this.state.lastRecursiveRefreshData)) {
            return false;
        };

        return _.isEqual(this.state.current_path, this.props.node.path) &&
            _.isEqual(this.state.recursive_sync_dir_version, this.props.version);
    }

    downloadFile = path=>{
        // Make a copy
        let new_path = [...path];

        // The accessor is the first top level.
        let accessor = new_path.shift();

        api.post("v1/VFSDownloadFile", {
            client_id: this.props.client.client_id,
            accessor: accessor,
            components: new_path,
        }, this.source.token).then(response => {
            let flow_id = response.data.flow_id;
            if(!flow_id) {
                return;
            }
            this.props.bumpVersion();
        });

    }

    startVfsRefreshOperation = () => {
        this.setState({current_path: this.props.node.path,
                       sync_dir_version: this.props.version});

        let path = this.props.node.path || [];
        api.post("v1/VFSRefreshDirectory", {
            client_id: this.props.client.client_id,
            vfs_components: path,
            depth: 0
        }, this.source.token).then((response) => {
            // If there is an error with calling the API cancel the
            // spin immediately. Otherwise wait for the directory version to update.
            if (!response.data || response.data.cancel ||
                !response.data.flow_id) {
                this.setState({sync_dir_version: this.props.version});
                return;
            }
        });
    }

    renderContextMenu() {
        return <Menu id="download-menu-id">
                  <Item onClick={e=>{
                      this.downloadFile(e.props.value);
                  }}>
                    <FontAwesomeIcon className="context-icon"
                                     icon="download"/>
                    {T("Download from client")}
                  </Item>
               </Menu>;
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
            <>
              <ButtonGroup className="float-right vfs-toolbar">
                { this.shouldSpinSyncDir() ?
                  <ToolTip tooltip={T("Currently refreshing")}>
                    <Button onClick={this.startVfsRefreshOperation}
                            variant="default">
                      <FontAwesomeIcon icon="spinner" spin/>
                    </Button>
                  </ToolTip>
                  :
                  <ToolTip
                    placement="bottom"
                    tooltip={T("Refresh this directory (sync its listing with the client)")}>
                    <Button disabled={!this.props.node.path}
                            onClick={this.startVfsRefreshOperation}
                            variant="default">
                      <FontAwesomeIcon icon="folder-open"/>
                    </Button>
                  </ToolTip>
                }
                { this.shouldSpinRecursiveSyncDir() ?
                  <ToolTip
                    placement="bottom"
                    tooltip={T("Currently refreshing from the client")}>
                    <Button onClick={this.cancelRecursiveRefresh}
                            variant="default">
                      <FontAwesomeIcon icon="spinner" spin/>
                      <span className="button-label">{T("Synced")} {(
                          this.state.lastRecursiveRefreshData.total_collected_rows || 0) + " files"}
                      </span>
                      <span className="button-label"><FontAwesomeIcon icon="stop"/></span>
                    </Button>
                  </ToolTip>  :
                  <ToolTip
                    placement="bottom"
                    tooltip={T("Recursively refresh this directory (sync its listing with the client)")}>
                    <Button disabled={!this.props.node.path}
                            onClick={this.startRecursiveVfsRefreshOperation}
                            variant="default">
                      <FontAwesomeIcon icon="folder-open"/>
                      <span className="recursive-list-button">R</span>
                    </Button>
                  </ToolTip>
                }

                { this.state.lastRecursiveDownloadFlowId ?
                  <ToolTip
                    placement="bottom"
                    tooltip={T("Currently fetching files from the client")}>
                    <Button onClick={this.cancelRecursiveDownload}
                            variant="default">
                      <FontAwesomeIcon icon="spinner" spin/>
                      <span className="button-label">Downloaded {
                          (this.state.lastRecursiveDownloadData.total_uploaded_files || 0) + " files (" +
                              (this.state.lastRecursiveDownloadData.total_uploaded_bytes/1024/1024 ||
                               0).toFixed(0) + "Mb)"}
                      </span>
                      <span className="button-label"><FontAwesomeIcon icon="stop"/></span>
                    </Button>
                  </ToolTip>  :
                  <ToolTip
                    placement="bottom"
                    tooltip={T("Recursively download this directory from the client")}>
                    <Button onClick={()=>this.setState({showDownloadAllDialog: true})}
                            disabled={_.isEmpty(this.props.node && this.props.node.path)}
                            variant="default">
                      <FontAwesomeIcon icon="file-download"/>
                      <span className="recursive-list-button">R</span>
                      &nbsp;
                    </Button>
                  </ToolTip>
                }

                <ToolTip tooltip={T("View Collection")}>
                  <Link to={"/collected/" +this.props.client.client_id +
                            "/" + this.props.node.flow_id + "/overview"}
                        role="button"
                        className={classNames({
                            "btn": true,
                            "btn-tooltip": true,
                            "btn-default": true,
                            "disabled":  !this.props.node.flow_id,
                        })}>
                    <FontAwesomeIcon icon="eye"/>
                  </Link>
                </ToolTip>
                <ToolTip tooltip={T("Stats Toggle")}>
                  <Button variant="default"
                          onClick={this.props.collapseToggle}>
                    <FontAwesomeIcon icon="expand"/>
                  </Button>
                </ToolTip>
                <ToolTip tooltip={T("Prepare Download")}>
                  <Button onClick={()=>this.setState({showExportDialog: true})}
                          variant="default">
                    <FontAwesomeIcon icon="file-export" />
                  </Button>
                </ToolTip>
              </ButtonGroup>
            </>
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

        let timestamp_renderer = (cell, row) => {
                return <div className="no-break">
                         <VeloTimestamp usec={cell}/>
                       </div>;
        };

        let nobreak_renderer = (cell, row) => {
                return <div className="no-break">
                         {cell}
                       </div>;
        };

        let row_filter = row=>{
            if(row._Idx === this.state.selectedTableIdx) {
                this.props.updateCurrentSelectedRow(row);
            };
            return row;
        };

        let renderers = {
            "mtime": timestamp_renderer,
            "atime": timestamp_renderer,
            "ctime": timestamp_renderer,
            "btime": timestamp_renderer,
            "Mode": nobreak_renderer,
            "Name": (cell, row) => {
                return <div className="min-width">{cell}</div>;
            },
            "Download": (cell, row) => {
                let result = [];
                if (cell) {
                        if (cell.in_flight) {
                            result.push( <FontAwesomeIcon
                                   className="hint-icon fa-spin"
                                   key="1" icon="spinner"/>);
                        } else if (cell.error) {
                            result.push( <FontAwesomeIcon
                                   className="hint-icon"
                                   key="1" icon="exclamation"/>);
                        } else if (!_.isEmpty(cell.components)) {
                            result.push( <FontAwesomeIcon
                                   className="hint-icon"
                                   key="1" icon="save"/>);
                        }
                }

                let fn_btime = row["_Data"] && row._Data["fn_btime"];
                let btime = row["btime"];
                if (btime < fn_btime) {
                    result.push( <FontAwesomeIcon
                                   key="2"
                                   className="file-stomped hint-icon"
                                   icon="clock"/>);
                }
                let new_path = [...this.props.node.path];
                new_path.push(row.Name);

                return <DownloadContextMenu value={new_path}>
                         <div className="file-hints">
                           {result}
                         </div>
                       </DownloadContextMenu>;
            },
            "Size": (cell, row) => {
                let result = parseInt(cell/1024/1024);
                let value = cell;
                let suffix = "";
                if (_.isFinite(result) && result > 0) {
                    suffix = "Mb";
                    value = result.toFixed(0);
                } else {
                    result = parseInt(cell /1024);
                    if (_.isFinite(result) && result > 0) {
                        suffix = "Kb";
                        value = result.toFixed(0);
                    } else {
                        if (_.isFinite(cell)) {
                            suffix = "";
                            value = cell.toFixed(0);
                        }
                    }
                }

                return <div className="number">{value + suffix}</div>;
            },
        };

        return (
            <>
              { this.renderContextMenu() }
              { this.state.showDownloadAllDialog &&
                <DownloadAllDialog
                  node={this.props.node}
                  client={this.props.client}
                  onCancel={()=>this.setState({showDownloadAllDialog: false})}
                  onClose={this.startRecursiveVfsDownloadOperation}
                />}
              { this.state.showExportDialog &&
                <ExportDialog
                  node={this.props.node}
                  client={this.props.client}
                  onCancel={()=>this.setState({showExportDialog: false})}
                  onClose={()=>this.setState({showExportDialog: false})}
                />}
              <div className="fill-parent no-margins selectable">
                <VeloPagedTable
                  url="v1/VFSListDirectoryFiles"
                  renderers={renderers}
                  version={{
                      version: this.props.version,
                  }}
                  params={{
                      client_id: this.props.client.client_id,
                      flow_id: this.props.node.flow_id,
                      artifact: "System.VFS.ListDirectory/Listing",
                      vfs_components: this.props.node.path,
                  }}
                  selectRow={ selectRow }
                  toolbar={toolbar}
                  initial_page_size={5}
                  row_filter={row_filter}
                />
              </div>
            </>
        );
    }
}

export default withRouter(VeloFileList);
