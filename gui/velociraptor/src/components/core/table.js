import "./table.css";

import _ from 'lodash';

import 'react-bootstrap-table2-paginator/dist/react-bootstrap-table2-paginator.min.css';
import ToolkitProvider from 'react-bootstrap-table2-toolkit';

import React, { Component } from 'react';
import PropTypes from 'prop-types';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { textFilter } from 'react-bootstrap-table2-filter';
import { SettingsButton } from '../core/ace.js';
import { Type } from 'react-bootstrap-table2-editor';
import BootstrapTable from 'react-bootstrap-table-next';
import paginationFactory from 'react-bootstrap-table2-paginator';
import Dropdown from 'react-bootstrap/Dropdown';
import Button from 'react-bootstrap/Button';
import ButtonGroup from 'react-bootstrap/ButtonGroup';
import Modal from 'react-bootstrap/Modal';
import Navbar from 'react-bootstrap/Navbar';
import VeloTimestamp from "../utils/time.js";
import VeloNotImplemented from '../core/notimplemented.js';
import VeloAce from '../core/ace.js';
import VeloValueRenderer from '../utils/value.js';
import api from '../core/api-service.js';

// Shows the InspectRawJson modal dialog UI.
export class InspectRawJson extends Component {
    static propTypes = {
        rows: PropTypes.array,
    }

    state = {
        show: false,
        ace: {},
    }

    aceConfig = (ace) => {
        ace.setOptions({
            wrap: true,
            autoScrollEditorIntoView: true,
            minLines: 10,
            readOnly: true,
            maxLines: 100000,
        });

        ace.resize();

        // Hold a reference to the ace editor.
        this.setState({ace: ace});
    };

    render() {
        let rows = [];
        let max_rows = this.props.rows.length;
        if (max_rows > 100) {
            max_rows = 100;
        }

        for(var i=0;i<max_rows;i++) {
            let copy = Object.assign({}, this.props.rows[i]);
            delete copy["_id"];
            rows.push(copy);
        }

        let serialized = JSON.stringify(rows, null, 2);

        return (
            <>
              <Button variant="default"
                      title="Inspect Raw JSON"
                      onClick={() => this.setState({show: true})} >
                <FontAwesomeIcon icon="binoculars"/>
              </Button>
              <Modal show={this.state.show}
                     className="full-height"
                     enforceFocus={false}
                     scrollable={true}
                     dialogClassName="modal-90w"
                     onHide={(e) => this.setState({show: false})}>
                <Modal.Header closeButton>
                  <Modal.Title>Raw Response JSON</Modal.Title>
                </Modal.Header>

                <Modal.Body>
                  <VeloAce text={serialized}
                           mode="json"
                           aceConfig={this.aceConfig} />
                </Modal.Body>

                <Modal.Footer>
                  <Navbar className="w-100 justify-content-between">
                    <ButtonGroup className="float-left">
                      <SettingsButton ace={this.state.ace}/>
                    </ButtonGroup>

                    <ButtonGroup className="float-right">
                      <Button variant="secondary"
                              onClick={() => this.setState({show: false})} >
                        Close
                      </Button>
                    </ButtonGroup>
                  </Navbar>
                </Modal.Footer>
              </Modal>
            </>
        );
    };
}

// Toggle columns on or off - helps when the table is very wide.
export const ColumnToggleList = (e) => {
    const [open, setOpen] = React.useState(false);
    const onToggle = (isOpen, ev, metadata) => {
        if (metadata.source === "select") {
            setOpen(true);
            return;
        }
        setOpen(isOpen);
    };

    const { columns, onColumnToggle, toggles } = e;
    let enabled_columns = [];

    let buttons = columns.map((column, idx) => {
        if (!column.text) {
            return <React.Fragment key={idx}></React.Fragment>;
        }
        let hidden = toggles[column.dataField];
        if (!hidden) {
            enabled_columns.push(column);
        }
        return <Dropdown.Item
                 key={ column.dataField }
                 eventKey={column.dataField}
                 active={!hidden}
                 onSelect={c=>onColumnToggle(c)}
                 title="Show/Hide Columns"
               >
                 { column.text }
               </Dropdown.Item>;
    });

    return (
        <>
          <Dropdown show={open} onToggle={onToggle}>
            <Dropdown.Toggle variant="default" id="dropdown-basic">
              <FontAwesomeIcon icon="columns"/>
            </Dropdown.Toggle>

            <Dropdown.Menu>
              { _.isEmpty(enabled_columns) ?
                <Dropdown.Item
                  onSelect={()=>{
                      _.each(columns, c=>{
                          if(toggles[c.dataField]){
                              onColumnToggle(c.dataField);
                          }
                      });
                  }}>
                  Set All
                </Dropdown.Item> :
                <Dropdown.Item
                  onSelect={()=>{
                      _.each(columns, c=>{
                          if(!toggles[c.dataField]){
                              onColumnToggle(c.dataField);
                          }
                      });
                  }}>
                  Clear All
                </Dropdown.Item> }
              <Dropdown.Divider />
              { buttons }
            </Dropdown.Menu>
          </Dropdown>
        </>
    );
};

export const sizePerPageRenderer = ({
  options,
  currSizePerPage,
  onSizePerPageChange
}) => (
  <div className="btn-group" role="group">
    {
      options.map((option) => {
        const isSelect = currSizePerPage === `${option.page}`;
        return (
          <button
            key={ option.text }
            type="button"
            onClick={ () => onSizePerPageChange(option.page) }
            className={ `btn ${isSelect ? 'btn-secondary' : 'btn-default'}` }
          >
            { option.text }
          </button>
        );
      })
    }
  </div>
);

class VeloTable extends Component {
    static propTypes = {
        rows: PropTypes.array,
        columns: PropTypes.array,

        // A dict containing renderers for each column.
        renderers: PropTypes.object,

        // A unique name we use to identify this table. We use this
        // name to save column preferences in the application user
        // context.
        name: PropTypes.string,
    }

    state = {
        download: false,
        toggles: {},
    }

    componentDidMount = () => {
        let toggles = {};
        // Hide columns that start with _
        _.each(this.props.columns, c=>{
            toggles[c] = c[0] === '_';
        });

        this.setState({toggles: toggles});
    }

    defaultFormatter = (cell, row, rowIndex) => {
        return <VeloValueRenderer value={cell}/>;
    }

    render() {
        if (!this.props.rows || !this.props.columns) {
            return <div></div>;
        }

        let rows = this.props.rows;

        let columns = [{dataField: '_id', hidden: true}];
        for(var i=0;i<this.props.columns.length;i++) {
            var name = this.props.columns[i];
            let definition ={ dataField: name, text: name};
            if (this.props.renderers && this.props.renderers[name]) {
                definition.formatter = this.props.renderers[name];
            } else {
                definition.formatter = this.defaultFormatter;
            }

            if (this.state.toggles[name]) {
                definition["hidden"] = true;
            }

            columns.push(definition);
        }

        // Add an id field for react ordering.
        for (var j=0; j<rows.length; j++) {
            rows[j]["_id"] = j;
        }
        return (
            <div className="velo-table">
              <ToolkitProvider
                bootstrap4
                keyField="_id"
                data={ rows }
                columns={ columns }
                toggles={this.state.toggles}
                columnToggle
            >
            {
                props => (
                    <div className="col-12">
                      <VeloNotImplemented
                        show={this.state.download}
                        resolve={() => this.setState({download: false})}
                      />
                      <Navbar className="toolbar">
                        <ButtonGroup>
                          <ColumnToggleList { ...props.columnToggleProps }
                                            onColumnToggle={(c)=>{
                                                // Do not make a copy
                                                // here because set
                                                // state is not
                                                // immediately visible
                                                // and this will be
                                                // called for each
                                                // column.
                                                let toggles = this.state.toggles;
                                                toggles[c] = !toggles[c];
                                                this.setState({toggles: toggles});
                                            }}
                                            toggles={this.state.toggles} />
                          <InspectRawJson rows={this.props.rows} />
                        </ButtonGroup>
                      </Navbar>
                      <div className="row col-12">
                        <BootstrapTable
                          { ...props.baseProps }
                          hover
                          condensed
                          keyField="_id"
                          headerClasses="alert alert-secondary"
                          bodyClasses="fixed-table-body"
                          toggles={this.state.toggles}
                          pagination={ paginationFactory({
                              showTotal: true,
                              sizePerPageRenderer
                          }) }
                        />
                      </div>
                    </div>
                )
            }
              </ToolkitProvider>
            </div>
        );
    }
};

export default VeloTable;

const int_regex = /^-?[0-9]+$/;

export function PrepareData(value) {
    var rows = [];
    let columns = value.columns || [];
    let value_rows = value.rows || [];
    for (var i=0; i < value_rows.length; i++) {
        var row = value_rows[i].cell;
        var new_row = {};
        for (var j=0; j<columns.length; j++) {
            var cell = j > row.length ? "" : row[j];
            var column = columns[j];

            // A bit of a hack for now, this represents an object.
            if (cell[0] === "{" || cell[0] === "[") {
                cell = JSON.parse(cell);
            } else if(cell.match(int_regex)) {
                cell = parseInt(cell);
            } else if(cell[0] === " ") {
                cell = cell.substr(1);
            }

            new_row[column] = cell;
        }
        rows.push(new_row);
    }

    return {columns: value.columns, rows: rows};
};

export function headerFormatter(column, colIndex, { sortElement, filterElement }) {
    let result = (
        // Not a real table but I cant figure out the css
        // right now so we do it old school.
        <table className="notebook-filter">
          <tbody>
            <tr>
              { column.filter ?
                <td>{ filterElement }</td> :
                <td>{ column.text }</td> }
              <td className="sort-element">{ sortElement }</td>
            </tr>
          </tbody>
        </table>
    );
    return result;
}

export function sortCaret(order, column) {
    if (!order) return <FontAwesomeIcon icon="sort"/>;
    else if (order === 'asc') return (
        <FontAwesomeIcon icon="sort-up"/>
    );
    else if (order === 'desc') return (
        <FontAwesomeIcon icon="sort-down"/>
    );

    return null;
}

export function formatColumns(columns) {
    _.each(columns, (x) => {
        x.headerFormatter=headerFormatter;
        if (x.sort) {
            x.sortCaret = sortCaret;
        }
        if (!x.editable) {
            x.editable = false;
        } else {
            x.editor = {
             type: Type.TEXTAREA,
            };
        }
        if (x.filtered) {
            x.filter = textFilter({
                placeholder: x.text,
                caseSensitive: false,
                delay: 10,
            });
        }
        if (x.sortNumeric) {
            x.sortFunc= (a, b, order, dataField) => {
                if (order === 'asc') {
                    return b - a;
                }
                return a - b; // desc
            };
        }
        switch(x.type) {
        case "mb":
            x.formatter=(cell, row) => {
                if (cell) {
                    return (cell /1024/1024).toFixed(0);
                }
                return <></>;
            };
            x.type = null;
            break;
        case "timestamp":
            x.formatter= (cell, row) => {
                return <VeloTimestamp usec={cell / 1000}/>;
            };
            x.type = null;
            break;

        case "nano_timestamp":
            x.formatter= (cell, row) => {
                return <VeloTimestamp usec={cell / 1000000}/>;
            };
            x.type = null;
            break;

        case "download":
            x.formatter= (cell, row) =>{
                if (row.complete) {
                    return <a href={api.base_path + row.path}  target="_blank" download
                              rel="noopener noreferrer">{cell}</a>;
                };
                return <>
                         <FontAwesomeIcon icon="spinner" spin/>
                         <span className="button-label">{cell}</span>
                       </>;
            };
            x.type = null;
            break;
        default: break;
        };
    });

    return columns;
}
