import './file-list.css';

import React, { Component } from 'react';
import PropTypes from 'prop-types';

import BootstrapTable from 'react-bootstrap-table-next';

import filterFactory, { textFilter } from 'react-bootstrap-table2-filter';

import Navbar from 'react-bootstrap/Navbar';
import Button from 'react-bootstrap/Button';

import { Join } from '../utils/paths.js';

import { withRouter }  from "react-router-dom";

import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';

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
        node.selected = row;
        this.props.updateCurrentNode(node);


        // Update the router with the new path.
        let vfs_path = [...this.props.node.path];
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
            classes: "row-selected",
            onSelect: this.updateCurrentFile,
            selected: [selected],
        };

        let toolbar = (
            <div className="navbar navbar-default toolbar"  >
              <div className="navbar-inner">
                <div className="navbar-form pull-left">
                  <div className="btn-group" role="group">
                    <button id="refresh-dir"
                            type="button"
                            ng-if="controller.mode == 'vfs_files'"
                            className="btn btn-default btn-rounded"
                            title="Refresh this directory (sync its listing with the client)"
                            ng-disabled="!controller.uiTraits.Permissions.collect_client ||
                            controller.lastRefreshOperationId"
                            ng-click="controller.startVfsRefreshOperation()">
                      <i className="fa fa-folder-open"></i>
                    </button>
                    <grr-recursive-list-button
                      client-id="controller.fileContext.clientId"
                      ng-if="controller.mode == 'vfs_files'"
                      ng-className="{'disabled': !controller.fileContext.clientId }"
                      file-path="controller.fileContext.selectedDirPath"
                      className="toolbar_icon">
                    </grr-recursive-list-button>

                    <grr-add-item-button
                      client-id="controller.fileContext.clientId"
                      file-path="controller.fileContext.selectedFilePath"
                      dir-path="controller.fileContext.selectedDirPath"
                      mode="controller.mode"
                    >
                    </grr-add-item-button>
                  </div>
                  <grr-vfs-files-archive-button
                    client-id="controller.fileContext.clientId"
                    file-path="controller.fileContext.selectedDirPath"
                    ng-if="controller.mode == 'vfs_files'"
                  >
                  </grr-vfs-files-archive-button>
                </div>

                <div className="navbar-form pull-right">
                  <div className="btn-group" role="group">
                    <button className="btn btn-default btn-rounded"
                            type="button"
                            ng-click="controller.showHelp()"
                            title="Help!">
                      <i className="fa fa-life-ring"></i>
                    </button>
                  </div>
                </div>
              </div>
            </div>
        );

        toolbar = (
            <Navbar className="toolbar">
              <Button variant="default"
                className="btn-rounded"
                title="Refresh this directory (sync its listing with the client)">
                <FontAwesomeIcon icon="folder-open"/>
              </Button>
            </Navbar>
        );

        if (!this.props.node || !this.props.node.raw_data) {
            return (
                <>
                { toolbar }
                <div className="fill-parent no-margins toolbar-margin">
                  No data available
                </div>;
                </>
            );
        }

        let columns = [
            {dataField: "Name", text: "",
             sort: true,
             filter: textFilter({
                 placeholder: "File names",
                 caseSensitive: false,
                 delay: 10,
             })},
            {dataField: "Size", text: "Size", sort: true},
            {dataField: "Mode", text: "Mode", sort: true},
            {dataField: "mtime", text: "mtime", sort: true},
            {dataField: "atime", text: "atime", sort: true},
            {dataField: "ctime", text: "ctime", sort: true}];

        return (
            <>
              { toolbar }
              <div className="fill-parent no-margins toolbar-margin">
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
