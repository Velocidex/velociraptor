import './file-list.css';

import React, { Component } from 'react';
import PropTypes from 'prop-types';

import BootstrapTable from 'react-bootstrap-table-next';

import filterFactory, { textFilter } from 'react-bootstrap-table2-filter';

import Navbar from 'react-bootstrap/Navbar';
import Button from 'react-bootstrap/Button';
import ButtonGroup from 'react-bootstrap/ButtonGroup';

import { Join } from '../utils/paths.js';

import { withRouter }  from "react-router-dom";

import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';

import { formatColumns } from "../core/table.js";

class VeloFileList extends Component {
    static propTypes = {
        node: PropTypes.object,
        client: PropTypes.object,
        updateCurrentNode: PropTypes.func,
    }

    updateCurrentFile = (row) => {
        // We store the currently selected row in the node. When the
        // user updates the row, we change the node's internal
        // references.
        let node = this.props.node;
        if (!node) {
            return;
        }

        node.selected = row;
        this.props.updateCurrentNode(node);


        let path = this.props.node.path || [];
        // Update the router with the new path.
        let vfs_path = [...path];
        vfs_path.push(row.Name);
        this.props.history.push("/vfs/" + this.props.client.client_id +
                                Join(vfs_path));
    }

    render() {
        // The node keeps track of which row is selected.
        let selected = this.props.node && this.props.node.selected && this.props.node.selected.Name;

        const selectRow = {
            mode: "radio",
            clickToSelect: true,
            hideSelectColumn: true,
            classes: "row-selected",
            onSelect: this.updateCurrentFile,
            selected: [selected],
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
                    No data available
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
