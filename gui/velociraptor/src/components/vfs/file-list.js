import './file-list.css';

import React, { Component } from 'react';
import PropTypes from 'prop-types';

import BootstrapTable from 'react-bootstrap-table-next';

import filterFactory, { textFilter } from 'react-bootstrap-table2-filter';

import Navbar from 'react-bootstrap/Navbar';
import Button from 'react-bootstrap/Button';
import ButtonGroup from 'react-bootstrap/ButtonGroup';

import { Join } from '../utils/paths.js';
import api from '../core/api-service.js';
import axios from 'axios';

import { withRouter }  from "react-router-dom";

import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';

import { formatColumns } from "../core/table.js";

const POLL_TIME = 2000;

class VeloFileList extends Component {
    static propTypes = {
        node: PropTypes.object,
        version: PropTypes.string,
        client: PropTypes.object,
        updateCurrentNode: PropTypes.func,
    }

    state = {
        lastRefreshOperationId: null,
    }

    componentWillUnmount() {
        if (this.source) {
            this.source.cancel("unmounted");
        }
        if (this.interval) {
            clearInterval(this.interval);
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

    startVfsRefreshOperation = () => {
        // Only allow one refresh at the time.
        if (this.state.lastRefreshOperationId) {
            return;
        }

        let path = this.props.node.path || [];
        api.post("api/v1/VFSRefreshDirectory", {
            client_id: this.props.client.client_id,
            vfs_path: Join(path),
            depth: 0
        }).then((response) => {
            this.setState({lastRefreshOperationId: response.data.flow_id});

            // Start polling for flow completion.
            this.source = axios.CancelToken.source();
            this.interval = setInterval(() => {
                api.get("api/v1/VFSStatDirectory", {
                    client_id: this.props.client.client_id,
                    vfs_path: Join(path),
                    flow_id: this.state.lastRefreshOperationId,
                }).then((response) => {
                    // The node is refreshed with the correct flow id, we can stop polling.
                    if (response.data.flow_id == this.state.lastRefreshOperationId) {
                        this.source.cancel("unmounted");
                        clearInterval(this.interval);
                        this.source = undefined;
                        this.interval = undefined;

                        // Force a tree refresh since this flow is done.
                        let node = this.props.node;
                        node.version = this.state.lastRefreshOperationId;
                        console.log(node);

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
                <Button title="Refresh this directory (sync its listing with the client)"
                        onClick={this.startVfsRefreshOperation}
                        variant="default">
                  <FontAwesomeIcon icon="folder-open"/>
                </Button>
                <Button title="Recursively refresh this directory (sync its listing with the client)"
                        onClick={this.startRecursiveVfsRefreshOperation}
                        variant="default">
                  <FontAwesomeIcon icon="folder-open"/>
                  <span className="recursive-list-button">R</span>
                </Button>
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
