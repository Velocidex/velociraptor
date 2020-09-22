import './file-list.css';

import React, { Component } from 'react';
import PropTypes from 'prop-types';

import BootstrapTable from 'react-bootstrap-table-next';
import filterFactory, { textFilter } from 'react-bootstrap-table2-filter'

import Navbar from 'react-bootstrap/Navbar';
import Button from 'react-bootstrap/Button';

import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';

class VeloFileList extends Component {
    static propTypes = {
        node: PropTypes.object,
        setSelectedRow: PropTypes.func,
    }
//{ JSON.stringify(this.props.node.raw_data)}

    render() {
        const selectRow = {
            mode: "radio",
            clickToSelect: true,
            classes: "row-selected",
            onSelect: function(row) {
                this.props.setSelectedRow(row);
            }.bind(this),
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
                  keyField="_FullPath"
                  bootstrap4
                  headerClasses="alert alert-secondary"
                  bodyClasses="fixed-table-body"
                  data={this.props.node.raw_data}
                  columns={columns}
                  filter={ filterFactory() }
                  selectRow={ selectRow }
                  rowStyle={(e) => console.log(e)}
                  onClick={(e) => console.log(e)}
                />
              </div>
            </>
        );
    }
}

export default VeloFileList;
