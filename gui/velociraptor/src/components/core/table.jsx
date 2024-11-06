import "./table.css";

import _ from 'lodash';

import 'react-bootstrap-table2-paginator/dist/react-bootstrap-table2-paginator.min.css';
import ToolkitProvider from 'react-bootstrap-table2-toolkit/dist/react-bootstrap-table2-toolkit.min';

import React, { Component } from 'react';
import PropTypes from 'prop-types';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import BootstrapTable from 'react-bootstrap-table-next';
import paginationFactory from 'react-bootstrap-table2-paginator';
import Dropdown from 'react-bootstrap/Dropdown';
import Button from 'react-bootstrap/Button';
import ButtonGroup from 'react-bootstrap/ButtonGroup';
import Modal from 'react-bootstrap/Modal';
import Navbar from 'react-bootstrap/Navbar';
import VeloTimestamp from "../utils/time.jsx";
import URLViewer from "../utils/url.jsx";
import VeloAce, { SettingsButton } from '../core/ace.jsx';
import VeloValueRenderer from '../utils/value.jsx';
import { NavLink } from "react-router-dom";
import ClientLink from '../clients/client-link.jsx';
import { HexViewPopup } from '../utils/hex.jsx';
import ToolTip from '../widgets/tooltip.jsx';
import T from '../i8n/i8n.jsx';
import TreeCell from './tree-cell.jsx';
import ContextMenu from '../utils/context.jsx';
import PreviewUpload from '../widgets/preview_uploads.jsx';
import filterFactory, { textFilter } from 'react-bootstrap-table2-filter';
import { JSONparse } from '../utils/json_parse.jsx';
import Download from "../widgets/download.jsx";
import VeloLog from "../widgets/logs.jsx";


// Shows the InspectRawJson modal dialog UI.
export class InspectRawJson extends Component {
    static propTypes = {
        rows: PropTypes.array,
        start: PropTypes.number,
        env: PropTypes.object,
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
        if (!this.state.show) {
            return <ToolTip tooltip={T("Inspect Raw JSON")}>
                     <Button variant="default"
                      onClick={() => this.setState({show: true})} >
                       <FontAwesomeIcon icon="binoculars"/>
                     </Button>
                   </ToolTip>;
        }

        let rows = [];
        let start = this.props.start || 0;
        let max_rows = 100;

        if (max_rows > this.props.rows.length) {
            max_rows = this.props.rows.length;
        }

        for(var i=start;i<start + max_rows;i++) {
            let copy = Object.assign({}, this.props.rows[i]);
            delete copy["_id"];
            rows.push(copy);
        }
        let serialized = JSON.stringify(rows, null, 2);

        return (
            <>
              <Modal show={this.state.show}
                     className="full-height"
                     enforceFocus={false}
                     scrollable={true}
                     dialogClassName="modal-90w"
                     onHide={(e) => this.setState({show: false})}>
                <Modal.Header closeButton>
                  <Modal.Title>{T("Raw Response JSON")}</Modal.Title>
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
                        {T("Close")}
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
    const onToggle = (isOpen, metadata) => {
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
               >
                 { column.text }
               </Dropdown.Item>;
    });

    return (
        <ToolTip tooltip={T("Show/Hide Columns")}>
          <Dropdown show={open}
                    onSelect={c=>{
                        onColumnToggle(c);
                      }}
                    onToggle={onToggle}>
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
                  {T("Set All")}
                </Dropdown.Item> :
                <Dropdown.Item
                  onClick={()=>{
                      _.each(columns, c=>{
                          if(!toggles[c.dataField]){
                              onColumnToggle(c.dataField);
                          }
                      });
                  }}>
                  {T("Clear All")}
                </Dropdown.Item> }
              <Dropdown.Divider />
              { buttons }
            </Dropdown.Menu>
          </Dropdown>
        </ToolTip>
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
        headers: PropTypes.object,

        column_renderers: PropTypes.object,

        // If set we do not render any toolbar
        no_toolbar: PropTypes.bool,
    }

    state = {
        toggles: {},
        from_page: 0,
        page_size: 10,
    }

    componentDidMount = () => {
        let toggles = {};
        // Hide columns that start with _
        _.each(this.props.columns, c=>{
            toggles[c] = c[0] === '_';
        });

        this.setState({toggles: toggles});
    }

    defaultFormatter = (cell, row, env) => {
        return <VeloValueRenderer value={cell}/>;
    }

    pageChange = (page, size)=>{
        this.setState({from_page: page-1, page_size: size});
    }

    pageSizeChange = (size) => {
        this.setState({page_size: size});
    }

    customTotal = (from, to, size) => {
        return <span className="react-bootstrap-table-pagination-total">
                 {T("TablePagination", from, to, size)}
               </span>;
    };

    // Calculate the visible columns in the current page. This is
    // needed to support tables with many columns which change on each
    // page.
    calculateColumns = ()=>{
        let start_page = this.state.from_page || 0;
        let page_size = this.state.page_size || 10;
        let columns = [];
        for(let i=start_page * page_size; i<(start_page + 1)*page_size;i++) {
            if (this.props.rows.length < i) {
                break;
            };
            _.forOwn(this.props.rows[i], (v, k)=>{
                if (k==="_id") { return };

                if(!_.find(columns, x=>x===k)) {
                    columns.push(k);
                };
            });
        }
        return columns;
    }

    render() {
        let start = (this.state.from_page || 0) * (this.state.page_size || 10);

        if (!this.props.rows) {
            return <div></div>;
        }

        let rows = this.props.rows;
        let column_names = this.props.columns || this.calculateColumns();
        if (_.isEmpty(column_names)) {
            return <div></div>;
        }

        let columns = [{dataField: '_id', text: "id", hidden: true}];
        for(var i=0;i<column_names.length;i++) {
            var name = column_names[i];
            var header = (this.props.headers || {})[name] || name;
            let definition ={ dataField: name, text: header};
            if (this.props.column_renderers && this.props.column_renderers[name]) {
                definition = this.props.column_renderers[name];
            }

            if (this.props.renderers && this.props.renderers[name]) {
                definition.formatter = this.props.renderers[name];
            } else if(!definition.formatter) {
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
                          { !this.props.no_toolbar &&
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
                                <InspectRawJson rows={this.props.rows}
                                                start={start}/>
                              </ButtonGroup>
                            </Navbar>
                          }
                      <div className="row col-12">
                        <BootstrapTable
                          { ...props.baseProps }
                          hover
                          condensed
                          keyField="_id"
                          headerClasses="alert alert-secondary"
                          bodyClasses="fixed-table-body"
                          toggles={this.state.toggles}
                          filter={filterFactory()}
                          pagination={ paginationFactory({
                              showTotal: true,
                              paginationTotalRenderer: this.customTotal,
                              sizePerPageRenderer,
                              onPageChange: this.pageChange,
                              onSizePerPageChange: this.pageSizeChange,
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

// The JSON response from the server is encoded as a strict protobuf
// with string cell values. Here we expand it into arbitrary JSON objects.

// TODO: This needs to be changed on the server to deliver a proper JSON response.
export function PrepareData(value) {
    let rows = [];
    let columns = value.columns || [];
    let value_rows = value.rows || [];
    for (let i=0; i < value_rows.length; i++) {
        let new_row = {};
        let json_row = value_rows[i].json;
        if (json_row) {
            let parsed = JSONparse(json_row);
            if (parsed && _.isArray(parsed)) {
                for (let j=0; j<columns.length; j++) {
                    let cell = j >= parsed.length ? null : parsed[j];
                    let column = columns[j];
                    new_row[column] = cell;
                }
                rows.push(new_row);
                continue;
            }
        }
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

// Returns a formatter by type.
export function getFormatter(column_type, text) {
    switch(column_type) {
    case "number":
        return (cell, row) => <span className="right-align">{cell}</span>;

    case "mb":
        return (cell, row) => {
            let result = parseInt(cell/1024/1024);
            let value = cell;
            let suffix = "";
            if (_.isFinite(result) && result > 0) {
                suffix = "Mb";
                value = parseInt(result);
            } else {
                result = parseInt(cell /1024);
                if (_.isFinite(result) && result > 0) {
                    suffix = "Kb";
                    value = parseInt(result);
                } else {
                    if (_.isFinite(cell)) {
                        suffix = "b";
                        value = parseInt(cell);
                    }
                }
            }
            return <ToolTip tooltip={cell}>
                     <span className="number right-align">
                       {value} {suffix}
                     </span>
                   </ToolTip>;
        };
    case "timestamp":
    case "nano_timestamp":
        return (cell, row) => {
            return <VeloTimestamp usec={cell}/>;
        };

    case "nobreak":
        return (cell, row) => {
            return <div className="no-break">{cell}</div>;
        };

    case "tree":
        return (cell, row) => {
            if (_.isObject(cell)) {
                return <TreeCell
                         name={text}
                         data={cell}/>;
            };
            return cell;
        };

        // A URL can be formatted as a markdown URL: [desc](url)
        // or can be a JSON object {url:"...", desc:"..."}
    case "url":
        return (cell, row) => {
            if(_.isObject(cell)) {
                return <URLViewer url={cell.url} desc={cell.desc}/>;
            }
            return <URLViewer url={cell}/>;
        };

    case "url_internal":
        return (cell, row) => {
            if(_.isObject(cell)) {
                return <URLViewer internal={true}
                             url={cell.url} desc={cell.desc}/>;
            }
            return <URLViewer url={cell} internal={true}/>;
        };

    case "safe_url":
        return (cell, row) => {
            return <URLViewer url={cell} safe={true}/>;
        };

    case "flow":
        return (cell, row) => {
            let client_id = row["ClientId"];
            if (!client_id) {
                return cell;
            };
            return <NavLink
                     tabIndex="0"
                     id={cell}
                     to={"/collected/" + client_id + "/" + cell}>{cell}
                   </NavLink>;
        };

    case "collapsed":
        return (cell, row) => {
            return <VeloValueRenderer
                    value={cell} collapsed={true}/>;
        };

    case "download":
        return (cell, row) => {
            // Ideally this is a UploadResponse object described
            // in /uploads/api.go. Such an object is emitted by
            // the uploads() VQL plugin.
            if(_.isObject(cell)) {
                let description = cell.Path;
                let fs_components = cell.Components || [];
                return <Download fs_components={fs_components}
                                 text={description}
                                 filename={description}/>;
            }

            let components = row._Components;
            let description = cell;
            let filename = row.client_path;

            return <Download fs_components={components}
                             text={description}
                             filename={filename}/>;
        };

    case "preview_upload":
    case "upload_preview":
        return (cell, row, env) => {
            let new_env = Object.assign({}, env || {});

            // If the row has a more updated client id and flow id
            // use them, otherwise use the ones from the query
            // env. For example when this component is viewed in a
            // hunt notebook we require the client id and flow id
            // to be in the table. But when viewed in the client
            // notebook we can use the client id and flow id from
            // the notebook env.
            if(row.ClientId) {
                new_env.client_id = row.ClientId;
            }
            if(row.FlowId) {
                new_env.flow_id = row.FlowId;
            }
            return <PreviewUpload
                     env={new_env}
                     upload={cell}/>;
        };

    case "client":
    case "client_id":
        return (cell, row) => {
            return <ClientLink client_id={cell}/>;
        };

    case "hex":
        return (cell, row) => {
            if (!cell.substr) return <></>;
            let bytearray = [];
            for (let c = 0; c < cell.length; c += 2) {
                let term = cell.substr(c, 2);
                if (term.match(/[0-9a-fA-F]{2}/)) {
                    bytearray.push(parseInt(term, 16));
                } else {
                    c--;
                }
            }
            return <ContextMenu value={cell}>
                     <HexViewPopup byte_array={bytearray}/>
                   </ContextMenu>;
        };
    case "base64hex":
    case "base64":
        return (cell, row) => {
            try {
                let binary_string = atob(cell);
                var len = binary_string.length;
                var bytes = new Uint8Array(len);
                for (var i = 0; i < len; i++) {
                    bytes[i] = binary_string.charCodeAt(i);
                }
                return <ContextMenu value={cell}>
                         <HexViewPopup byte_array={bytes}/>
                       </ContextMenu>;
            } catch(e) {
                return <></>;
            };
        };

    case "string":
    case "bool":
    case "date":
    case undefined:
        return (cell, row) => <ContextMenu value={cell}>
                                <VeloValueRenderer value={cell} />
                              </ContextMenu>;

    case "hidden":
        return (cell, row) =><></>;

    case "log":
        return (cell, row) =><VeloLog value={cell}/>;

    default:
        console.log("Unsupported column type " + column_type);
        return (cell, row) => <VeloValueRenderer value={cell} />;
    };
}



export function formatColumns(columns, env, column_formatter) {
    _.each(columns, (x) => {
        x.headerFormatter = column_formatter || headerFormatter;
        if(x.no_transformation) {
            x.headerFormatter = headerFormatter;
        }
        if (x.sort) {
            x.sortCaret = sortCaret;
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
        case "number":
            if (!_.isObject(x.style)) {
                x.style = {};
            }
            x.style.textAlign = "right";
            x.type = null;
            break;

        case "mb":
            x.formatter=(cell, row) => {
                let result = parseInt(cell/1024/1024);
                let value = cell;
                let suffix = "";
                if (_.isFinite(result) && result > 0) {
                    suffix = "Mb";
                    value = parseInt(result);
                } else {
                    result = parseInt(cell /1024);
                    if (_.isFinite(result) && result > 0) {
                        suffix = "Kb";
                        value = parseInt(result);
                    } else {
                        if (_.isFinite(cell)) {
                            suffix = "b";
                            value = parseInt(cell);
                        }
                    }
                }
                return <ToolTip tooltip={cell}>
                         <span className="number">
                         {value} {suffix}
                         </span>
                       </ToolTip>;
            };
            if (!_.isObject(x.style)) {
                x.style = {};
            }
            x.style.textAlign = "right";
            x.type = null;
            break;
        case "timestamp":
        case "nano_timestamp":
            x.formatter= (cell, row) => {
                return <VeloTimestamp usec={cell}/>;
            };
            x.type = null;
            break;

        case "nobreak":
            x.formatter= (cell, row) => {
                return <div className="no-break">{cell}</div>;
            };
            x.type = null;
            break;

        case "tree":
            x.formatter= (cell, row) => {
                if (_.isObject(cell)) {
                    return <TreeCell
                                name={x.text}
                                data={cell}/>;
                };
                return cell;
            };
            x.type = null;
            break;

            // A URL can be formatted as a markdown URL: [desc](url)
            // or can be a JSON object {url:"...", desc:"..."}
        case "url":
            x.formatter = (cell, row) => {
                if(_.isObject(cell)) {
                    return <URLViewer url={cell.url} desc={cell.desc}/>;
                }
                return <URLViewer url={cell}/>;
            };
            x.type = null;
            break;

        case "url_internal":
            x.formatter = (cell, row) => {
                if(_.isObject(cell)) {
                    return <URLViewer internal={true}
                                      url={cell.url} desc={cell.desc}/>;
                }
                return <URLViewer url={cell} internal={true} />;
            };
            x.type = null;
            break;


        case "safe_url":
            x.formatter = (cell, row) => {
                return <URLViewer url={cell} safe={true}/>;
            };
            x.type = null;
            break;

        case "flow":
            x.formatter = (cell, row) => {
                let client_id = row["ClientId"];
                if (!client_id) {
                    return cell;
                };
                return <NavLink
                         tabIndex="0"
                         id={cell}
                         to={"/collected/" + client_id + "/" + cell}>{cell}
                       </NavLink>;
            };
            x.type = null;
            break;

        case "collapsed":
            x.formatter = (cell, row) => {
                return <VeloValueRenderer
                         value={cell} collapsed={true}/>;
            };
            x.type = null;
            break;

        case "download":
            x.formatter = (cell, row) => {
                // Ideally this is a UploadResponse object described
                // in /uploads/api.go. Such an object is emitted by
                // the uploads() VQL plugin.
                if(_.isObject(cell)) {
                    let description = cell.Path;
                    let fs_components = cell.Components || [];
                    return <Download fs_components={fs_components}
                                     text={description}
                                     filename={description}/>;
                }

                let components = row._Components;
                let description = cell;
                let filename = row.client_path;

                return <Download fs_components={components}
                                 text={description}
                                 filename={filename}/>;
            };
            x.type = null;
            break;

        case "preview_upload":
        case "upload_preview":
            x.formatter = (cell, row) => {
                let new_env = Object.assign({}, env);

                // If the row has a more updated client id and flow id
                // use them, otherwise use the ones from the query
                // env. For example when this component is viewed in a
                // hunt notebook we require the client id and flow id
                // to be in the table. But when viewed in the client
                // notebook we can use the client id and flow id from
                // the notebook env.
                if(row.ClientId) {
                    new_env.client_id = row.ClientId;
                }
                if(row.FlowId) {
                    new_env.flow_id = row.FlowId;
                }
                return <PreviewUpload
                         env={new_env}
                         upload={cell}/>;
            };
            x.type = null;
            break;

        case "client":
        case "client_id":
            x.formatter = (cell, row) => {
                return <ClientLink client_id={cell}/>;
            };
            x.type = null;
            break;

        case "hex":
            x.formatter = (cell, row) => {
                if (!cell.substr) return <></>;
                let bytearray = [];
                for (let c = 0; c < cell.length; c += 2) {
                    let term = cell.substr(c, 2);
                    if (term.match(/[0-9a-fA-F]{2}/)) {
                        bytearray.push(parseInt(term, 16));
                    } else {
                        c--;
                    }
                }
                return <ContextMenu value={cell}>
                         <HexViewPopup byte_array={bytearray}/>
                       </ContextMenu>;
            };
            x.type = null;
            break;

        case "base64hex":
        case "base64":
            x.formatter = (cell, row) => {
                try {
                    let binary_string = atob(cell);
                    var len = binary_string.length;
                    var bytes = new Uint8Array(len);
                    for (var i = 0; i < len; i++) {
                        bytes[i] = binary_string.charCodeAt(i);
                    }
                    return <ContextMenu value={cell}>
                             <HexViewPopup byte_array={bytes}/>
                           </ContextMenu>;
                } catch(e) {
                    return <></>;
                };
            };
            x.type = null;
            break;


            // Types supported by the underlying BootstrapTable - just
            // pass them on.
        case "string":
        case "bool":
        case "date":
        case undefined:
            break;

        case "hidden":
            x.type = null;
            break;

        default:
            console.log("Unsupported column type " + x.type);
            x.type = null;
            break;
        };
    });

    return columns;
}
