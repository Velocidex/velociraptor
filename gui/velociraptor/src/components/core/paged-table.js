import "./table.css";

import _ from 'lodash';

import 'react-bootstrap-table2-paginator/dist/react-bootstrap-table2-paginator.min.css';
import ToolkitProvider from 'react-bootstrap-table2-toolkit';

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

import { InspectRawJson, ColumnToggleList, sizePerPageRenderer, PrepareData } from './table.js';



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
            placeholder="Goto Page"
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
    }

    componentDidMount = () => {
        this.source = axios.CancelToken.source();
        this.fetchRows();
    }

    componentWillUnmount() {
        this.source.cancel();
    }

    componentDidUpdate(prevProps, prevState, snapshot) {
        if (!_.isEqual(prevProps.params, this.props.params)) {
            this.setState({start_row: 0, toggles: {}, columns: []});
        };

        if (!_.isEqual(prevProps.params, this.props.params) ||
            prevState.start_row !== this.state.start_row ||
            prevState.page_size !== this.state.page_size) {
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


    fetchRows = () => {
        if (_.isEmpty(this.props.params) || !this.props.params.artifact) {
            this.setState({loading: false});
            return;
        }

        let params = Object.assign({}, this.props.params);
        params.start_row = this.state.start_row;
        params.rows = this.state.page_size;

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
            return <div className="no-content">
                     No Data Available.
                   </div>;
        }


        let rows = this.state.rows;
        let column_names = [];
        let columns = [{dataField: '_id', hidden: true}];
        for(var i=0;i<this.state.columns.length;i++) {
            var name = this.state.columns[i];
            let definition ={ dataField: name, text: name};
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

            columns.push(definition);
        }

        // Add an id field for react ordering.
        for (var j=0; j<rows.length; j++) {
            rows[j]["_id"] = j;
        }


        let total_size = this.state.total_size || 0;
        if (total_size < 0 && !_.isEmpty(this.state.rows)) {
            total_size = this.state.rows.length + this.state.start_row;
            if (total_size > 500) {
                total_size = 500;
            }
        }

        let downloads = Object.assign({columns: column_names}, this.props.params);
        return (
            <div className="velo-table full-height"> <Spinner loading={this.state.loading} />
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
                        </ButtonGroup>
                      </Navbar>
                      <div className="row col-12">
                        <BootstrapTable
                          { ...props.baseProps }
                          hover
                          remote
                          condensed
                          noDataIndication="Table is Empty"
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
