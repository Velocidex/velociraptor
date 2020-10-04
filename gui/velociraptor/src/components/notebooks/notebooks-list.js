import "./notebooks-list.css";

import React from 'react';
import PropTypes from 'prop-types';
import _ from 'lodash';
import VeloTimestamp from "../utils/time.js";
import filterFactory, { textFilter } from 'react-bootstrap-table2-filter';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import BootstrapTable from 'react-bootstrap-table-next';

import ButtonGroup from 'react-bootstrap/ButtonGroup';
import Button from 'react-bootstrap/Button';
import Navbar from 'react-bootstrap/Navbar';

import { formatColumns } from "../core/table.js";


export default class NotebooksList extends React.Component {
    static propTypes = {
        notebooks: PropTypes.array,
        selected_notebook: PropTypes.object,
        setSelectedNotebook: PropTypes.func.isRequired,
    };

    render() {
        if (!this.props.notebooks || !this.props.notebooks.length) {
            return <div>No Data available</div>;
        }

        let columns = formatColumns([
            {dataField: "notebook_id", text: "NotebookId"},
            {dataField: "name", text: "Name",
             sort: true, filtered: true },
            {dataField: "description", text: "Description",
             sort: true, filtered: true },
            {dataField: "created_time", text: "Creation Time",
             sort: true, formatter: (cell, row) => {
                return <VeloTimestamp usec={cell * 1000}/>;
            }},
            {dataField: "modified_time", text: "Modified Time",
             sort: true, formatter: (cell, row) => {
                 return <VeloTimestamp usec={cell * 1000}/>;
             }},
            {dataField: "creator", text: "Creator"},
            {dataField: "collaborators", text: "Collaborators",
             sort: true, formatter: (cell, row) => {
                 return _.map(cell, function(item, idx) {
                     return <div key={idx}>{item}</div>;
                 });
             }},
        ]);

        let selected_notebook = this.props.selected_notebook &&
            this.props.selected_notebook.notebook_id;
        const selectRow = {
            mode: "radio",
            clickToSelect: true,
            hideSelectColumn: true,
            classes: "row-selected",
            onSelect: function(row) {
                this.props.setSelectedNotebook(row);
            }.bind(this),
            selected: [selected_notebook],
        };

        return (
            <>
              <Navbar className="toolbar">
                <ButtonGroup>
                  <Button title="NewNotebook"
                          onClick={this.newNotebook}
                          variant="default">
                    <FontAwesomeIcon icon="plus"/>
                  </Button>

                  <Button title="Delete Notebook"
                          onClick={this.deleteNotebook}
                          variant="default">
                    <FontAwesomeIcon icon="trash"/>
                  </Button>

                  <Button title="Edit Notebook"
                          onClick={this.editNotebook}
                          variant="default">
                    <FontAwesomeIcon icon="wrench"/>
                  </Button>
                  <Button title="ExportNotebook"
                          onClick={this.exportNotebook}
                          variant="default">
                    <FontAwesomeIcon icon="download"/>
                  </Button>
                </ButtonGroup>
              </Navbar>
              <div className="fill-parent no-margins toolbar-margin">
                <BootstrapTable
                  hover
                  condensed
                  keyField="notebook_id"
                  bootstrap4
                  headerClasses="alert alert-secondary"
                  bodyClasses="fixed-table-body"
                  data={this.props.notebooks}
                  columns={columns}
                  selectRow={ selectRow }
                  filter={ filterFactory() }
                />
              </div>
            </>
        );
    }
};
