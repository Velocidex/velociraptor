import "./table.css";

import _ from 'lodash';

import './paged-table.css';

import 'react-bootstrap-table2-paginator/dist/react-bootstrap-table2-paginator.min.css';
import ToolkitProvider from 'react-bootstrap-table2-toolkit/dist/react-bootstrap-table2-toolkit.min';

import axios from 'axios';

import React, { Component } from 'react';
import PropTypes from 'prop-types';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import qs from 'qs';
import BootstrapTable from 'react-bootstrap-table-next';
import paginationFactory from 'react-bootstrap-table2-paginator';
import Button from 'react-bootstrap/Button';
import Pagination from 'react-bootstrap/Pagination';
import Form from 'react-bootstrap/Form';
import ButtonGroup from 'react-bootstrap/ButtonGroup';
import Navbar from 'react-bootstrap/Navbar';
import VeloValueRenderer from '../utils/value.js';
import Spinner from '../utils/spinner.js';
import api from '../core/api-service.js';
import VeloTimestamp from "../utils/time.js";
import ClientLink from '../clients/client-link.js';
import HexView from '../utils/hex.js';
import TableTransformDialog from './table-transform-dialog.js';
import T from '../i8n/i8n.js';

import {
    InspectRawJson, ColumnToggleList,
    sizePerPageRenderer, PrepareData,
    formatColumns,
} from './table.js';



const pageListRenderer = ({
    pages,
    currentPage,
    totalRows,
    pageSize,
    onPageChange
}) => {
    // just exclude <, <<, >>, >
    const pageWithoutIndication = pages.filter(p => typeof p.page !== 'string');
    let totalPages = parseInt(totalRows / pageSize);

    // Only allow changing to a page if there are any rows in that
    // page.
    if (totalPages * pageSize + 1 > totalRows) {
        totalPages--;
    }
    return (
        <Pagination>
          <Pagination.First onClick={()=>onPageChange(0)}/>
          {
              pageWithoutIndication.map((p, idx)=>(
                <Pagination.Item
                  key={idx}
                  active={p.active}
                  onClick={ () => onPageChange(p.page) } >
                  { p.page }
                </Pagination.Item>
            ))
          }
          <Pagination.Last onClick={()=>onPageChange(totalPages)}/>
          <Form.Control
            as="input"
            className="pagination-form"
            placeholder={T("Goto Page")}
            spellCheck="false"
            value={currentPage || ""}
            onChange={e=> {
                let page = parseInt(e.currentTarget.value || 0);
                if (page >= 0 && page < totalPages) {
                    onPageChange(page);
                }
            }}/>

        </Pagination>
    );
};


class VeloPagedTable extends Component {
    static propTypes = {
        // Params to the GetTable API call.
        params: PropTypes.object,

        // A dict containing renderers for each column.
        renderers: PropTypes.object,

        // A unique name we use to identify this table. We use this
        // name to save column preferences in the application user
        // context.
        name: PropTypes.string,

        // The URL Handler to fetch the table content. Defaults to
        // "v1/GetTable".
        url: PropTypes.string,


        // When called will cause the table to be recalculated.
        refresh: PropTypes.func,

        // A version to force refresh of the table.
        version: PropTypes.object,
    }

    state = {
        rows: [],
        columns: [],

        download: false,
        toggles: {},
        start_row: 0,
        page_size: 10,

        // If the table has an index this will be the total number of
        // rows in the table. If it is -1 then we dont know the total
        // number.
        total_size: 0,
        loading: true,

        // A transform applied on the basic table.
        transform: {},

        show_transform_dialog: false,
    }

    componentDidMount = () => {
        this.source = axios.CancelToken.source();
        this.fetchRows();
    }

    componentWillUnmount() {
        this.source.cancel();
    }

    componentDidUpdate(prevProps, prevState, snapshot) {
        if (!_.isEqual(prevProps.version, this.props.version)) {
            this.fetchRows();
        };

        if (!_.isEqual(prevProps.params, this.props.params)) {
            this.setState({
                start_row: 0,
                transform: {},
                toggles: {},
                columns: [],
            });
        };

        if (!_.isEqual(prevProps.params, this.props.params) ||
            prevState.start_row !== this.state.start_row ||
            prevState.page_size !== this.state.page_size ||
            !_.isEqual(prevState.transform, this.state.transform)) {
            this.fetchRows();
        }
    }

    defaultFormatter = (cell, row, rowIndex) => {
        return <VeloValueRenderer value={cell}/>;
    }

    getColumnRenderer = (column, column_types) => {
        if (!_.isArray(column_types)) {
            return this.defaultFormatter;
        }

        for (let i=0; i<column_types.length; i++) {
            if (column === column_types[i].name) {
                let type = column_types[i].type;
                switch (type) {
                case "base64":
                    return (cell, row, rowIndex)=>{
                        let decoded = cell.slice(0,1000);
                        try {
                            decoded = atob(cell);
                        } catch(e) {};

                        return <HexView data={decoded} height="2"/>;
                    };
                case "timestamp":
                    return (cell, row, rowIndex)=><VeloTimestamp usec={cell * 1000} iso={cell}/>;

                case "client_id":
                    return (cell, row, rowIndex)=><ClientLink client_id={cell}/>;

                default:
                    return this.defaultFormatter;
                }
            }
        }
        return this.defaultFormatter;
    }

    getTransformed = ()=>{
        let result = [];
        let transform = this.state.transform || {};
        if (transform.filter_column) {
            result.push(
                <Button key="1"
                  title="Transformed"  disabled={true}
                  variant="outline-dark">
                  { transform.filter_column } ( {transform.filter_regex} )
                  <span className="transform-button">
                    <FontAwesomeIcon icon="filter"/>
                  </span>
                </Button>
            );
        }

        if (transform.sort_column) {
            result.push(
                <Button key="2"
                  title="Transformed"  disabled={true}
                  variant="outline-dark">
                  {transform.sort_column}
                  <span className="transform-button">
                    {
                        transform.sort_direction === "Ascending" ?
                            <FontAwesomeIcon icon="sort-alpha-up"/> :
                        <FontAwesomeIcon icon="sort-alpha-down"/>
                    }
                  </span>
                </Button>
            );
        }

        return result;
    }


    fetchRows = () => {
        if (_.isEmpty(this.props.params)) {
            this.setState({loading: false});
            return;
        }

        let params = Object.assign({}, this.props.params);
        Object.assign(params, this.state.transform);
        params.start_row = this.state.start_row;
        params.rows = this.state.page_size;
        params.sort_direction = params.sort_direction === "Ascending";

        let url = this.props.url || "v1/GetTable";

        this.source.cancel();
        this.source = axios.CancelToken.source();

        this.setState({loading: true});
        api.get(url, params, this.source.token).then((response) => {
            if (response.cancel) {
                return;
            }

            let pageData = PrepareData(response.data);
            let toggles = Object.assign({}, this.state.toggles);
            let columns = pageData.columns;
            if (_.isEmpty(this.state.toggles) && !_.isUndefined(columns)) {
                let hidden = 0;

                // Hide columns that start with _
                _.each(columns, c=>{
                    if (c[0] === '_') {
                        toggles[c] = true;
                        hidden++;
                    } else {
                        toggles[c] = false;
                    }
                });

                // If all the columns are hidden, then just show them
                // all beause we need to show something.
                if (hidden === columns.length) {
                    toggles = {};
                }

            }
            this.setState({loading: false,
                           total_size: parseInt(response.data.total_rows || 0),
                           rows: pageData.rows,
                           toggles: toggles,
                           column_types: response.data.column_types,
                           columns: columns });
        }).catch(() => {
            this.setState({loading: false, rows: [], columns: []});
        });
    }

    render() {
        if (_.isEmpty(this.state.columns) && this.state.loading) {
            return <>
                     <Spinner loading={this.state.loading} />
                     <div className="no-content">
                        Loading....
                     </div>
                   </>;
        }

        if (_.isEmpty(this.state.columns)) {
            if (this.props.refresh) {
                return <div className="no-content">
                         <div>No Data Available.</div>
                         <Button variant="default" onClick={this.props.refresh}>
                           Recalculate <FontAwesomeIcon icon="sync"/>
                         </Button>
                       </div>;
            }
            return <div className="no-content">
                     <div>No Data Available.</div>
                   </div>;
        }

        let table_options = this.props.params.TableOptions || {};
        let rows = this.state.rows;
        let column_names = [];
        let columns = [{dataField: '_id', hidden: true}];
        for(var i=0;i<this.state.columns.length;i++) {
            let name = this.state.columns[i];
            let definition ={ dataField: name, text: T(name)};
            if (this.props.renderers && this.props.renderers[name]) {
                definition.formatter = this.props.renderers[name];
            } else {
                definition.formatter = this.getColumnRenderer(
                    name, this.state.column_types);
            }

            if (this.state.toggles[name]) {
                definition.hidden = true;
            } else {
                column_names.push(name);
            }

            // Allow the user to specify the column type for
            // rendering. This can be done in the notebook by simply
            // setting a VQL variable:
            // LET ColumnTypes = dict(BootTime="timestamp")
            // Or in an artifact using the column_types field
            definition.type = table_options[name];

            // If the user does not override the table options in VQL,
            // the column types are set from the artifact's
            // definition.
            _.each(this.state.column_types, x=>{
                if (x.name === name && !definition.type) {
                    definition.type = x.type;
                }
            });

            columns.push(definition);
        }

        // Add an id field for react ordering.
        for (var j=0; j<rows.length; j++) {
            rows[j]["_id"] = j;
        }

        columns = formatColumns(columns);

        let total_size = this.state.total_size || 0;
        if (total_size < 0 && !_.isEmpty(this.state.rows)) {
            total_size = this.state.rows.length + this.state.start_row;
            if (total_size > 500) {
                total_size = 500;
            }
        }
        let transformed = this.getTransformed();
        let downloads = Object.assign({columns: column_names}, this.props.params);
        return (
            <div className="velo-table full-height"> <Spinner loading={this.state.loading} />
              { this.state.show_transform_dialog &&
                <TableTransformDialog
                  columns={this.state.columns}
                  transform={this.state.transform}
                  setTransform={x=>{
                      this.setState({
                          transform: x,
                          start_row: 0,
                      });
                  }}
                  onClose={()=>this.setState({show_transform_dialog: false})}
                />
              }
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
                          <InspectRawJson rows={this.state.rows} />
                          <Button variant="default"
                                  target="_blank" rel="noopener noreferrer"
                                  title="Download JSON"
                                  href={api.base_path + "/api/v1/DownloadTable?"+
                                        qs.stringify(downloads,  {indices: false}) } >
                            <FontAwesomeIcon icon="download"/>
                          </Button>
                          <Button variant="default"
                                  target="_blank" rel="noopener noreferrer"
                                  title="Download CSV"
                                  href={api.base_path + "/api/v1/DownloadTable?download_format=csv&"+
                                        qs.stringify(downloads,  {indices: false}) } >
                            <FontAwesomeIcon icon="file-csv"/>
                          </Button>
                          <Button variant="default"
                                  onClick={()=>this.setState({
                                      show_transform_dialog: true,
                                  })}
                                  title="Transform Table">
                            <FontAwesomeIcon icon="filter"/>
                          </Button>
                        </ButtonGroup>
                        { transformed.length > 0 &&
                          <ButtonGroup className="float-right">
                            { transformed }
                          </ButtonGroup>
                        }
                      </Navbar>
                      <div className="row col-12">
                        <BootstrapTable
                          { ...props.baseProps }
                          hover
                          remote
                          condensed
                          noDataIndication={T("Table is Empty")}
                          keyField="_id"
                          headerClasses="alert alert-secondary"
                          bodyClasses="fixed-table-body"
                          toggles={this.state.toggles}
                          onTableChange={(type, { page, sizePerPage }) => {
                              this.setState({start_row: page * sizePerPage});
                          }}
                          pagination={ paginationFactory({
                              showTotal: true,
                              sizePerPage: this.state.page_size,
                              totalSize: total_size,
                              currSizePerPage: this.state.page_size,
                              onSizePerPageChange: value=>{
                                  this.setState({page_size: value});
                              },
                              pageStartIndex: 0,
                              pageListRenderer: ({pages, onPageChange})=>pageListRenderer({
                                  totalRows: total_size,
                                  pageSize: this.state.page_size,
                                  pages: pages,
                                  currentPage: this.state.start_row / this.state.page_size,
                                  onPageChange: onPageChange}),
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

export default VeloPagedTable;
