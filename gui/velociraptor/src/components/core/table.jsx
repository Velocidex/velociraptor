import "./table.css";

import _ from 'lodash';

import React, { Component } from 'react';
import PropTypes from 'prop-types';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
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
import FlowLink from '../flows/flow-link.jsx';
import { HexViewPopup } from '../utils/hex.jsx';
import ToolTip from '../widgets/tooltip.jsx';
import T from '../i8n/i8n.jsx';
import TreeCell from './tree-cell.jsx';
import ContextMenu from '../utils/context.jsx';
import PreviewUpload from '../widgets/preview_uploads.jsx';
import { JSONparse } from '../utils/json_parse.jsx';
import Download from "../widgets/download.jsx";
import VeloLog from "../widgets/logs.jsx";
import ColumnResizer from "./column-resizer.jsx";
import Table from 'react-bootstrap/Table';
import { TablePaginationControl, ColumnToggle } from './paged-table.jsx';
import NumberFormatter from '../utils/number.jsx';
import LogLevel from '../utils/log_level.jsx';

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

// The JSON response from the server is encoded as a strict protobuf
// with string cell values. Here we expand it into arbitrary JSON objects.
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

// Returns a formatter by type.
export function getFormatter(column_type, text) {
    switch(column_type) {
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
    case "flow_id":
        return (cell, row) => {
            let client_id = row["ClientId"];
            if (!client_id) {
                return cell;
            };
            return <FlowLink flow_id={cell} client_id={client_id}/>;
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

    case "number":
        return (cell, row) => {
            return <NumberFormatter value={cell}/>;
        };

        // A json object which is completely collapsed at all levels.
    case "json/0":
        return (cell, row) => <VeloValueRenderer
                                value={cell}
                                expand_map={{}} />;

        // A json object which is completely collapsed at all levels
        // except for the first level which is expanded.
    case "json/1":
        return (cell, row) => <VeloValueRenderer
                                value={cell}
                                expand_map={{0:1}} />;

    case "json/2":
        return (cell, row) => <VeloValueRenderer
                                value={cell}
                                expand_map={{0:1,1:1}} />;

    case "json/3":
        return (cell, row) => <VeloValueRenderer
                                value={cell}
                                expand_map={{0:1,1:1,2:1}} />;

    case "hidden":
        return (cell, row) =><></>;

    case "log":
        return (cell, row) =><VeloLog value={cell}/>;

    case "log_level":
        return (cell, row) =><LogLevel type={cell}/>;

    case "translated":
        return (cell, row) => { return T(cell); };

    default:
        console.log("Unsupported column type " + column_type);
        return (cell, row) => <VeloValueRenderer value={cell} />;
    };
}

class VeloTable extends Component {
    static propTypes = {
        rows: PropTypes.array,
        columns: PropTypes.array,

        // A unique name we use to identify this table. We use this
        // name to save column preferences in the application user
        // context.
        name: PropTypes.string,
        column_renderers: PropTypes.object,
        header_renderers: PropTypes.object,


        // If set we do not render any toolbar
        no_toolbar: PropTypes.bool,

        env: PropTypes.object,

        toolbar: PropTypes.object,

        onSelect: PropTypes.func,
        selected: PropTypes.func,
    }

    state = {
        columns: [],
        toggles: {},
        start_row: 0,
        page_size: 10,
        total_size: 0,
        column_widths: {},
    }

    componentDidMount = () => {
        let toggles = {};
        // Hide columns that start with _
        _.each(this.props.columns, c=>{
            toggles[c] = c[0] === '_';
        });

        this.setState({toggles: toggles, columns: this.props.columns});
    }

    activeColumns = ()=>{
        let res = [];
        _.each(this.state.columns, c=>{
            if(!this.state.toggles[c]) {
                res.push(c);
            }
        });
        return res;
    }

    renderHeader = (column, idx)=>{
        let styles = {};
        let col_width = this.state.column_widths[column];
        if (col_width) {
            styles = {
                minWidth: col_width,
                maxWidth: col_width,
                width: col_width,
            };
        }

        let header_renderers = this.props.header_renderers || {};
        let header = header_renderers[column] || column;
        if (_.isFunction(header)) {
            header = header();
        }

        return (
            <React.Fragment key={idx}>
              <th style={styles}
                  className={" draggable paged-table-header"}
                  onDragStart={e=>{
                      e.dataTransfer.setData("column", column);
                  }}
                  onDrop={e=>{
                      e.preventDefault();
                      this.swapColumns(e.dataTransfer.getData("column"), column);
                  }}
                  onDragOver={e=>{
                      e.preventDefault();
                      e.dataTransfer.dropEffect = "move";
                  }}
                  draggable="true">
                <span className="column-name">
                  { header }
                </span>
              </th>
              <ColumnResizer
                width={this.state.column_widths[column]}
                setWidth={x=>{
                    let column_widths = Object.assign(
                        {}, this.state.column_widths);
                    column_widths[column] = x;
                    this.setState({column_widths: column_widths});
                }}
              />
            </React.Fragment>
        );
    }

    defaultFormatter = (cell, row, rowIndex) => {
        let row_data = {};
        _.each(this.activeColumns(), x=>{
            row_data[x] = row[x];
        });
        return <VeloValueRenderer value={cell} row={row_data} />;
    }

    getColumnRenderer = column => {
        if(this.props.column_renderers &&
           this.props.column_renderers[column]) {
            return this.props.column_renderers[column] ||
                this.defaultFormatter;
        }
        return this.defaultFormatter;
    }

    renderCell = (column, row, rowIdx) => {
        let t = this.state.toggles[column];
        if(t) {return undefined;};

        let cell = row[column];
        let renderer = this.getColumnRenderer(column);

        return <React.Fragment key={column}>
                 <td>
                   { renderer(cell, row, this.props.env)}
                 </td>
                 <ColumnResizer
                   width={this.state.column_widths[column]}
                   setWidth={x=>{
                       let column_widths = Object.assign(
                           {}, this.state.column_widths);
                       column_widths[column] = x;
                       this.setState({column_widths: column_widths});
                   }}
                 />
               </React.Fragment>;
    };


    renderRow = (row, idx)=>{
        let start = (this.state.start_row || 0);
        let end = start + this.state.page_size;
        let total_size = this.props.rows && this.props.rows.length;
        if (end>total_size) {
            end=total_size;
        }

        if (idx >= end || idx < start) {
            return undefined;
        }

        let selected_cls = "";
        if(this.props.selected && this.props.selected(row)) {
            selected_cls = "row-selected";
        }

        return (
            <tr key={idx}
                onClick={()=>{
                    if(this.props.onSelect) {
                        this.props.onSelect(row, idx);
                    }
                }}
                className={selected_cls}>
              {_.map(this.activeColumns(), c=>this.renderCell(c, row, idx))}
            </tr>);
    }

    // Insert the to_col right before the from_col
    swapColumns = (from_col, to_col)=>{
        let new_columns = [];
        let from_seen = false;

        if (from_col === to_col) {
            return;
        }

        _.each(this.state.columns, x=>{
            if(x === to_col) {
                if (from_seen) {
                    new_columns.push(to_col);
                    new_columns.push(from_col);
                } else {
                    new_columns.push(from_col);
                    new_columns.push(to_col);
                }
            }

            if(x === from_col) {
                from_seen = true;
            }

            if(x !== from_col && x !== to_col) {
                new_columns.push(x);
            }
        });
        this.setState({columns: new_columns});
    }

    renderPaginator = (direction)=>{
        let total_size = this.props.rows && this.props.rows.length;
        return (
            <>
              <TablePaginationControl
                total_size={total_size}
                start_row={this.state.start_row}
                page_size={this.state.page_size}
                current_page={this.state.start_row / this.state.page_size}
                onRowChange={row_offset=>this.setState({start_row: row_offset})}
                onPageSizeChange={size=>this.setState({page_size: size})}
                direction={direction}
              />
            </>    );
    }

    render() {
        return (
            <>
              { !this.props.no_toolbar &&
                <Navbar className="toolbar">
                  <ButtonGroup>
                    <ColumnToggle onToggle={c=>{
                        // Do not make a copy here because set state
                        // is not immediately visible and this will be
                        // called for each column.
                        let toggles = this.state.toggles;
                        toggles[c] = !toggles[c];
                        this.setState({toggles: toggles});
                    }}
                                  headers={this.props.header_renderers}
                                  columns={this.state.columns}
                                  swapColumns={this.swapColumns}
                                  toggles={this.state.toggles} />

                  { this.renderPaginator() }
                  </ButtonGroup>
                  { this.props.toolbar || <></> }
                </Navbar>
              }

              <Table className="paged-table">
                <thead>
                  <tr className="paged-table-header">
                    {_.map(this.activeColumns(), this.renderHeader)}
                  </tr>
                </thead>
                <tbody className="fixed-table-body">
                  {_.map(this.props.rows, this.renderRow)}
                </tbody>
              </Table>
            </>
        );
    }
}

export default VeloTable;
