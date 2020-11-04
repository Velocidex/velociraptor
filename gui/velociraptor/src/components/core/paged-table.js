import "./table.css";

import _ from 'lodash';

import 'react-bootstrap-table2-paginator/dist/react-bootstrap-table2-paginator.min.css';
import ToolkitProvider from 'react-bootstrap-table2-toolkit';

import React, { Component } from 'react';
import PropTypes from 'prop-types';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';

import BootstrapTable from 'react-bootstrap-table-next';
import paginationFactory from 'react-bootstrap-table2-paginator';
import Button from 'react-bootstrap/Button';
import Pagination from 'react-bootstrap/Pagination';
import Form from 'react-bootstrap/Form';
import ButtonGroup from 'react-bootstrap/ButtonGroup';
import Navbar from 'react-bootstrap/Navbar';
import VeloNotImplemented from '../core/notimplemented.js';
import VeloValueRenderer from '../utils/value.js';
import Spinner from '../utils/spinner.js';
import api from '../core/api-service.js';

import { InspectRawJson, ColumnToggleList, sizePerPageRenderer, PrepareData } from './table.js';



const pageListRenderer = ({
    pages,
    currentPage,
    totalPages,
    onPageChange
}) => {
    // just exclude <, <<, >>, >
    const pageWithoutIndication = pages.filter(p => typeof p.page !== 'string');
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
    }

    state = {
        rows: [],
        columns: [],

        download: false,
        toggles: undefined,
        start_row: 0,
        page_size: 10,

        // If the table has an index this will be the total number of
        // rows in the table. If it is -1 then we dont know the total
        // number.
        total_size: 0,
        loading: false,
    }

    componentDidMount = () => {
        this.fetchRows();
    }

    componentDidUpdate(prevProps, prevState, snapshot) {
        if (!_.isEqual(prevProps.params, this.props.params) ||
            prevState.start_row !== this.state.start_row ||
            prevState.page_size !== this.state.page_size) {
            this.fetchRows();
        }
    }

    XXXcomponentDidMount = () => {
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


    fetchRows = () => {
        if (_.isEmpty(this.props.params)) {
            return;
        }

        let params = Object.assign({}, this.props.params);
        params.start_row = this.state.start_row;
        params.rows = this.state.page_size - 1;

        this.setState({loading: true});
        api.get("v1/GetTable", params).then((response) => {
            let pageData = PrepareData(response.data);

            let columns = pageData.columns;
            if (_.isUndefined(this.state.toggles)) {
                let toggles = {};

                // Hide columns that start with _
                _.each(this.props.columns, c=>{
                    toggles[c] = c[0] === '_';
                });

                this.setState({toggles: toggles});
            }

            this.setState({loading: false,
                           total_size: parseInt(response.data.total_rows || 0),
                           rows: pageData.rows,
                           columns: columns });
        }).catch(() => {
            this.setState({loading: false, rows: [], columns: []});
        });
    }

    render() {
        if (_.isEmpty(this.props.params) || _.isEmpty(this.state.columns)) {
            return <div className="no-content">
                     <Spinner loading={this.state.loading} />
                     No data available
                   </div>;
        }

        let rows = this.state.rows;

        let columns = [{dataField: '_id', hidden: true}];
        for(var i=0;i<this.state.columns.length;i++) {
            var name = this.state.columns[i];
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


        let total_size = this.state.total_size;
        if (total_size<0 && !_.isEmpty(this.state.rows)) {
            total_size = this.state.rows.length + this.state.start_row;
            if (total_size > 500) {
                total_size = 500;
            }
        }

        return (
            <div className="velo-table"> <Spinner loading={this.state.loading} />
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
                                                let toggles = this.state.toggles;
                                                toggles[c] = !toggles[c];
                                                this.setState({toggles: toggles});
                                            }}
                                            toggles={this.state.toggles} />
                          <InspectRawJson rows={this.state.rows} />
                          <Button variant="default"
                                  onClick={() => this.setState({download: true})} >
                            <FontAwesomeIcon icon="download"/>
                          </Button>
                        </ButtonGroup>
                      </Navbar>
                      <div className="row col-12">
                        <BootstrapTable
                          { ...props.baseProps }
                          hover
                          remote
                          condensed
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
                                  totalPages: total_size / this.state.page_size,
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
