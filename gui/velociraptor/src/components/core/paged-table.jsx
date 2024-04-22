import "./table.css";

import _ from 'lodash';

import './paged-table.css';

import 'react-bootstrap-table2-paginator/dist/react-bootstrap-table2-paginator.min.css';
import ToolkitProvider from 'react-bootstrap-table2-toolkit/dist/react-bootstrap-table2-toolkit.min';

import {CancelToken} from 'axios';

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
import VeloValueRenderer from '../utils/value.jsx';
import Spinner from '../utils/spinner.jsx';
import api from '../core/api-service.jsx';
import VeloTimestamp from "../utils/time.jsx";
import ClientLink from '../clients/client-link.jsx';
import HexView from '../utils/hex.jsx';
import StackDialog from './stack.jsx';

import T from '../i8n/i8n.jsx';
import UserConfig from '../core/user.jsx';

import {
    InspectRawJson, ColumnToggleList,
    sizePerPageRenderer, PrepareData,
    formatColumns,
} from './table.jsx';


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
        if (totalPages<0) {
            totalPages = 0;
        }
    }
    return (
        <Pagination>
          <Pagination.First
            disabled={currentPage===0}
            onClick={()=>onPageChange(0)}/>
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
          <Pagination.Last
            disabled={currentPage===totalPages}
            onClick={()=>onPageChange(totalPages)}/>
          <Form.Control
            as="input"
            className="pagination-form"
            placeholder={T("Goto Page")}
            spellCheck="false"
            id="goto-page"
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

class ColumnFilter extends Component {
    static propTypes = {
        column:  PropTypes.string,
        transform: PropTypes.object,
        setTransform: PropTypes.func,
    }

    state = {
        edit_visible: false,
        edit_filter: "",
    }

    componentDidMount = () => {
        this.updateFiltersFromTransform();
    }

    componentDidUpdate(prevProps, prevState, snapshot) {
        if (!_.isEqual(prevProps.transform, this.props.transform)) {
            this.updateFiltersFromTransform();
        }
    }

    updateFiltersFromTransform = () => {
        let transform = this.props.transform || {};
        let edit_filter = "";

        if(this.props.column === transform.filter_column) {
            edit_filter = transform.filter_regex;
        }
        this.setState({
            edit_filter: edit_filter,
            edit_visible: transform.editing === this.props.column,
        });
    }

    submitSearch = ()=>{
        let edit_filter = this.state.edit_filter;
        let transform = Object.assign({}, this.props.transform);
        transform.editing = "";

        if(edit_filter) {
            transform.filter_column = this.props.column;
            transform.filter_regex = this.state.edit_filter;
        } else {
            transform.filter_column = null;
            transform.filter_regex = null;
        }
        this.setState({
            edit_visible: false,
        });
        this.props.setTransform(transform);
    }

    render() {
        let classname = "hidden-edit";

        if(this.state.edit_visible) {
            return <Form onSubmit={()=>this.submitSearch()}>
                     <Form.Control
                       as="input"
                       className="visible"
                       placeholder={T("Regex")}
                       spellCheck="false"
                       value={this.state.edit_filter}
                       onChange={e=> {
                           this.setState({edit_filter: e.currentTarget.value});
                       }}/>
                   </Form>;
        }

        if (this.state.edit_filter) {
            classname = "visible";
        }

        return <Button
                 size="sm"
                 variant="outline-dark"
                 className={classname}
                 onClick={()=>{
                     this.setState({
                         edit_visible: true,
                     });
                     this.props.setTransform({editing: this.props.column});
                 }}>
                 <FontAwesomeIcon icon="filter"/>
                 { this.state.edit_filter }
               </Button>;
    }
}


class ColumnSort extends Component {
    static propTypes = {
        column:  PropTypes.string,
        transform: PropTypes.object,
        setTransform: PropTypes.func,
    }

    submitSort = next_dir=>{
        let transform = Object.assign({}, this.props.transform);
        transform.sort_column = this.props.column;
        transform.sort_direction = next_dir;;
        this.props.setTransform(transform);
    }

    render() {
        let icon = "sort";
        let next_dir = "Ascending";
        let classname = "hidden-edit";

        let sort_direction = "";
        if (this.props.transform &&
            this.props.transform.sort_column == this.props.column) {
            sort_direction = this.props.transform.sort_direction;
        }

        if (sort_direction === "Ascending") {
            icon = "arrow-up-a-z";
            next_dir = "Descending";
            classname = "visible";
        } else if (sort_direction === "Descending"){
            icon = "arrow-down-a-z";
            next_dir = "Ascending";
            classname = "visible";
        }


        return <Button
                 size="sm"
                 className={classname}
                 variant="outline-dark"
                 onClick={()=>this.submitSort(next_dir)}
               >
                 <FontAwesomeIcon icon={icon}/>
               </Button>;
    }
}


class VeloPagedTable extends Component {
    static contextType = UserConfig;

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

        // An environment that will be passed to the column renderers
        env: PropTypes.object,

        // When called will cause the table to be recalculated.
        refresh: PropTypes.func,

        // A version to force refresh of the table.
        version: PropTypes.object,

        translate_column_headers: PropTypes.bool,

        // If set we remove the option to filter/sort the table.
        no_transformations: PropTypes.bool,

        // A list of column names to prevent transforms on
        prevent_transformations: PropTypes.object,

        // An optional toolbar that can be passed to the table.
        toolbar: PropTypes.object,

        // If set we do not render any toolbar
        no_toolbar: PropTypes.bool,

        // If specified we notify that a transform is set.
        transform: PropTypes.object,
        setTransform: PropTypes.func,

        // A descriptor object for the selector.
        // https://react-bootstrap-table.github.io/react-bootstrap-table2/docs/row-select-props.html
        selectRow: PropTypes.object,

        initial_page_size: PropTypes.number,

        // Additional columns to add (should be formatted with a
        // custom renderer).
        extra_columns: PropTypes.array,
        columns:  PropTypes.object,

        // A callback that will be called for each row fetched.
        row_filter: PropTypes.func,

        // Control the class of each row
        row_classes: PropTypes.func,

        // If set we disable the spinner.
        no_spinner: PropTypes.bool,

        // If set we report table columns here for completion.
        completion_reporter: PropTypes.func,
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

        last_data: [],

        stack_path: [],

        showStackDialog: false,

        // Keep state for individual stacking transforms
        stack_transforms: {},
    }

    componentDidMount = () => {
        this.source = CancelToken.source();
        this.setState({page_size: this.props.initial_page_size || 10});

        this.fetchRows();
    }

    componentWillUnmount() {
        this.source.cancel();
    }

    componentDidUpdate(prevProps, prevState, snapshot) {
        if (this.props.transform &&
            !_.isEqual(prevProps.transform, this.props.transform)) {
            this.setState({transform: this.props.transform});
        }

        if (!_.isEqual(prevProps.version, this.props.version)) {
            this.fetchRows();
            return;
        }

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
                        } catch(e) {}

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

    // Render the transformed view at the top right of the
    // table. Shows the user what transforms are currnetly active.
    getTransformed = ()=>{
        let result = [];
        let transform = Object.assign({}, this.state.transform || {});
        if(_.isEmpty(transform) && !_.isEmpty(this.props.transform)) {
            Object.assign(transform, this.props.transform);
        }

        if (transform.filter_column) {
            result.push(
                <Button key="1"
                        data-tooltip={T("Transformed")}  disabled={true}
                        data-position="right"
                        className="btn-tooltip table-transformed"
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
                        data-tooltip={T("Transformed")}  disabled={true}
                        data-position="right"
                        className="btn-tooltip"
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
        let transform = Object.assign({}, this.state.transform || {});
        if(_.isEmpty(transform) && !_.isEmpty(this.props.transform)) {
            Object.assign(transform, this.props.transform);
        }

        Object.assign(params, transform);
        params.start_row = this.state.start_row || 0;
        if (params.start_row < 0) {
            params.start_row = 0;
        }
        params.rows = this.state.page_size;
        params.sort_direction = params.sort_direction === "Ascending";

        let url = this.props.url || "v1/GetTable";

        this.source.cancel();
        this.source = CancelToken.source();

        this.setState({loading: true});

        api.get(url, params, this.source.token).then((response) => {
            if (response.cancel) {
                return;
            }

            let stack_path = response.data && response.data.stack_path;
            let pageData = PrepareData(response.data);
            let toggles = Object.assign({}, this.state.toggles);
            let columns = pageData.columns;
            if (this.props.extra_columns) {
                columns = columns.concat(this.props.extra_columns);
            };

            if(this.props.completion_reporter) {
                this.props.completion_reporter(columns);
            }

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
            if (this.props.row_filter) {
                pageData.rows = _.map(pageData.rows, this.props.row_filter);
            }

            this.setState({loading: false,
                           total_size: parseInt(response.data.total_rows || 0),
                           rows: pageData.rows,
                           all_columns: pageData.columns,
                           toggles: toggles,
                           stack_path: stack_path || [],
                           column_types: response.data.column_types,
                           columns: columns });
        }).catch(() => {
            this.setState({loading: false, rows: [], columns: [], stack_path: []});
        });
    }

    customTotal = (from, to, size) => (
        <span className="react-bootstrap-table-pagination-total">
          {T("TablePagination", from, to, size)}
        </span>
    );

    // Update the transform specification between all the columns
    setTransform = transform=>{
        let new_transform = Object.assign({}, this.state.transform);
        new_transform = Object.assign(new_transform, transform);
        if(!_.isEqual(new_transform, this.state.transform)) {
            // When the transform changes filters we need to reset to
            // the first page because otherwise we might be past the
            // end of the table.
            if (new_transform.filter_column !== this.state.transform.filter_column ||
                new_transform.filter_regex !== this.state.transform.filter_regex) {
                this.setState({start_row: 0});
            }
            if(this.props.setTransform) {
                this.props.setTransform(transform);
            }
            this.setState({transform: new_transform});
        }
    }

    isColumnStacked = name=>{
        if (_.isEmpty(this.state.stack_path) ||
            this.state.stack_path.length < 3) {
            return false ;
        }
        return this.state.stack_path[this.state.stack_path.length-3] === name;
    }

    // Format the column headers. Columns have a sort and an filter button
    // which keep track of their own filtering and sorting states but
    // update the primary transform.
    headerFormatter = (column, colIndex) => {
        return (
            <table className="paged-table-header">
              <tbody>
                <tr>
                  <td>{ column.text }</td>
                  <td className="sort-element">
                    <ButtonGroup>
                      { this.isColumnStacked(column.text) &&
                        <Button variant="default"
                                target="_blank" rel="noopener noreferrer"
                                data-tooltip={T("Stack")}
                                data-position="right"
                                onClick={()=>this.setState({
                                    showStackDialog: column.text,
                                })}
                                className="btn-tooltip">
                          <FontAwesomeIcon icon="layer-group"/>
                          <span className="sr-only">{T("Stack")}</span>
                        </Button>
                      }
                      <ColumnSort column={column.dataField}
                                  transform={this.state.transform}
                                  setTransform={this.setTransform}
                      />

                      <ColumnFilter column={column.dataField}
                                    transform={this.state.transform}
                                    setTransform={this.setTransform}
                      />
                    </ButtonGroup>
                  </td>
                </tr>
              </tbody>
            </table>
        );
    }


    render() {
        let timezone = (this.context.traits &&
                        this.context.traits.timezone) || "UTC";

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
                let transformed = this.state.transform &&
                    (this.state.transform.filter_column ||
                     this.state.transform.filter_column);

                return <>
                         <div className="col-12">
                           { !this.props.no_toolbar &&
                              <Navbar className="toolbar">
                                { this.props.toolbar || <></> }
                              </Navbar> }
                           <div className="no-content">
                             <div>{T("No Data Available.")}</div>
                             <Button variant="default" onClick={this.props.refresh}>
                               {T("Recalculate")} <FontAwesomeIcon icon="sync"/>
                             </Button>
                             { transformed &&
                               <Button variant="default"
                                       onClick={()=>this.setState({transform: {}})}>
                                 {T("Clear")}
                                 <FontAwesomeIcon icon="filter"/>
                               </Button>
                             }
                           </div>
                         </div>
                       </>;

            }
            return <>
                     <div className="col-12">
                       { !this.props.no_toolbar &&
                         <Navbar className="toolbar">
                           { this.props.toolbar || <></> }
                         </Navbar> }
                       <div className="no-content">
                         <div>{T("No Data Available.")}</div>
                       </div>
                     </div>
                   </>;
        }

        let table_options = this.props.params.TableOptions || {};
        let rows = this.state.rows;
        let column_names = [];
        let columns = [{dataField: '_id', hidden: true}];
        let prop_columns = this.props.columns || {};
        for(var i=0;i<this.state.columns.length;i++) {
            let name = this.state.columns[i];

            let definition ={ dataField: name,
                              text: this.props.translate_column_headers ?
                              T(name) : name};

            if(_.isObject(prop_columns[name])) {
                Object.assign(definition, prop_columns[name]);
            }

            if (this.props.renderers && this.props.renderers[name]) {
                definition.formatter = this.props.renderers[name];
            } else {
                definition.formatter = this.getColumnRenderer(
                    name, this.state.column_types);
            }

            if (this.props.prevent_transformations &&
                this.props.prevent_transformations[name]) {
                definition.no_transformation = true;
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

        if (this.props.no_transformations) {
            columns = formatColumns(columns, this.props.env);
        } else {
            columns = formatColumns(columns, this.props.env, this.headerFormatter);
        }

        let total_size = this.state.total_size || 0;
        if (total_size < 0 && !_.isEmpty(this.state.rows)) {
            total_size = this.state.rows.length + this.state.start_row;
            if (total_size > 500) {
                total_size = 500;
            }
        }
        let transformed = this.getTransformed();
        let downloads = Object.assign({}, this.props.params);
        if (!_.isEqual(this.state.all_columns, column_names)) {
            downloads.columns = column_names;
        }
        return (
            <div className="velo-table full-height">
              <Spinner loading={!this.props.no_spinner && this.state.loading} />
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
                            <InspectRawJson rows={this.state.rows} />
                            <Button variant="default"
                                    target="_blank" rel="noopener noreferrer"
                                    data-tooltip={T("Download JSON")}
                                    data-position="right"
                                    className="btn-tooltip"
                                    href={api.href("/api/v1/DownloadTable",
                                                   Object.assign(downloads, {
                                                       timezone: timezone,
                                                       download_format: "json",
                                                   }), {internal: true})}>
                              <FontAwesomeIcon icon="download"/>
                              <span className="sr-only">{T("Download JSON")}</span>
                            </Button>
                            <Button variant="default"
                                    target="_blank" rel="noopener noreferrer"
                                    data-tooltip={T("Download CSV")}
                                    data-position="right"
                                    className="btn-tooltip"
                                    href={api.href("/api/v1/DownloadTable",
                                                   Object.assign(downloads, {
                                                       timezone: timezone,
                                                       download_format: "csv",
                                                   }), {internal: true})}>
                              <FontAwesomeIcon icon="file-csv"/>
                              <span className="sr-only">{T("Download CSV")}</span>
                            </Button>
                          </ButtonGroup>
                          { transformed.length > 0 &&
                            <ButtonGroup className="float-right">
                              { transformed }
                            </ButtonGroup>
                          }
                          { this.props.toolbar || <></> }
                        </Navbar> }
                      <div className="row col-12">
                        <BootstrapTable
                          { ...props.baseProps }
                          hover
                          remote
                          condensed
                          selectRow={ this.props.selectRow }
                          noDataIndication={T("Table is Empty")}
                          keyField="_id"
                          headerClasses="alert alert-secondary paged-table-header"
                          bodyClasses="fixed-table-body"
                          rowClasses={this.props.row_classes}
                          toggles={this.state.toggles}
                          onTableChange={(type, { page, sizePerPage }) => {
                              this.setState({start_row: page * sizePerPage});
                          }}
                          pagination={ paginationFactory({
                              showTotal: true,
                              sizePerPage: this.state.page_size,
                              totalSize: total_size,
                              paginationTotalRenderer: this.customTotal,
                              currSizePerPage: this.state.page_size,
                              onSizePerPageChange: value=>{
                                  this.setState({page_size: value});
                              },
                              pageStartIndex: 0,
                              page: parseInt(this.state.start_row / this.state.page_size),
                              pageListRenderer: ({pages, onPageChange})=>pageListRenderer({
                                  totalRows: total_size,
                                  pageSize: this.state.page_size,
                                  pages: pages,
                                  currentPage: parseInt(this.state.start_row / this.state.page_size),
                                  onPageChange: onPageChange}),
                              sizePerPageRenderer
                          }) }
                        />
                      </div>
                    </div>
                )
            }
              </ToolkitProvider>
              { this.state.showStackDialog &&
                <StackDialog
                  name={this.state.showStackDialog}
                  onClose={()=>this.setState({showStackDialog: false})}
                  navigateToRow={row=>this.setState({start_row: row})}
                  stack_path={this.state.stack_path}

                  transform={this.state.stack_transforms[
                      this.state.showStackDialog] || {}}
                  setTransform={t=>{
                      let stack_transforms = this.state.stack_transforms;
                      stack_transforms[this.state.showStackDialog] = t;
                      this.setState({stack_transforms: stack_transforms});
                  }}
                />
              }
            </div>
        );
    }
}

export default VeloPagedTable;
