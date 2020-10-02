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

        function headerFormatter(column, colIndex, { sortElement, filterElement }) {
            return (
                // Not a real table but I cant figure out the css
                // right now so we do it old school.
                <table className="notebook-filter">
                  <tbody>
                    <tr>
                      { column.filter && <td>{ filterElement }</td> }
                      { !column.filter && <td>{ column.text }</td> }
                      <td className="sort-element">{ sortElement }</td>
                    </tr>
                  </tbody>
                </table>
            );
        }

        function sortCaret(order, column) {
            if (!order) return <FontAwesomeIcon icon="sort"/>;
            else if (order === 'asc') return (
                <FontAwesomeIcon icon="sort-up"/>
            );
            else if (order === 'desc') return (
                <FontAwesomeIcon icon="sort-down"/>
            );

            return null;
        }

        let columns = [
            {dataField: "notebook_id", text: "NotebookId"},
            {dataField: "name", text: "Name",
             sort: true, sortCaret: sortCaret,
             headerFormatter: headerFormatter,
             filter: textFilter({
                 placeholder: "Name",
                 caseSensitive: false,
                 delay: 10,
             })},
            {dataField: "description", text: "Description",
             sort: true,sortCaret: sortCaret,
             headerFormatter: headerFormatter,
             filter: textFilter({
                 placeholder: "Description",
                 caseSensitive: false,
                 delay: 10,
             })},
            {dataField: "created_time", text: "Creation Time",
             sort: true, sortCaret: sortCaret,
             headerFormatter: headerFormatter,
            formatter: (cell, row) => {
                return <VeloTimestamp usec={cell * 1000}/>
            }},
            {dataField: "modified_time", text: "Modified Time",
             sort: true, sortCaret: sortCaret,
             headerFormatter: headerFormatter,
             formatter: (cell, row) => {
                 return <VeloTimestamp usec={cell * 1000}/>
             }},
            {dataField: "creator", text: "Creator",
             headerFormatter: headerFormatter,
            },
            {dataField: "collaborators", text: "Collaborators",
             sort: true, sortCaret: sortCaret,
             headerFormatter: headerFormatter,
             formatter: (cell, row) => {
                 return _.map(cell, function(item, idx) {
                     return <div key={idx}>{item}</div>;
                 });
             }},
        ];


        let selected_notebook = this.props.selected_notebook &&
            this.props.selected_notebook.notebook_id;
        const selectRow = {
            mode: "radio",
            clickToSelect: true,
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
