import "./table.css";

import _ from 'lodash';

import './paged-table.css';

import {CancelToken} from 'axios';

import { HotKeys } from "react-hotkeys";
import React, { Component } from 'react';
import PropTypes from 'prop-types';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import Button from 'react-bootstrap/Button';
import Form from 'react-bootstrap/Form';
import ButtonGroup from 'react-bootstrap/ButtonGroup';
import Navbar from 'react-bootstrap/Navbar';
import VeloValueRenderer from '../utils/value.jsx';
import api from '../core/api-service.jsx';
import ToolTip from '../widgets/tooltip.jsx';
import Table from 'react-bootstrap/Table';
import Dropdown from 'react-bootstrap/Dropdown';
import StackDialog from './stack.jsx';
import ColumnResizer from "./column-resizer.jsx";
import InputGroup from 'react-bootstrap/InputGroup';
import { serializeJSON } from '../utils/json_parse.jsx';
import { Link }  from "react-router-dom";

import T from '../i8n/i8n.jsx';
import UserConfig from '../core/user.jsx';

import {
    getFormatter,
    InspectRawJson,
    PrepareData,
} from './table.jsx';

let guid = 1;

const getID = ()=>{
    guid++;
    return "ID" + guid;
}


export class ColumnFilter extends Component {
    static propTypes = {
        column:  PropTypes.string,
        table_id: PropTypes.string,
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

    // The GUID should be unique for this column and table.
    guid = ()=>{
        return this.props.table_id + this.props.column;
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
        let tooltip = T("Filter");
        if(this.state.edit_visible) {
            tooltip = this.props.column;
            classname = "visible";
        }

        return (
            <ToolTip tooltip={tooltip}>
                <Form
                  onSubmit={e=>{
                      e.preventDefault();
                      e.stopPropagation();

                      if(this.state.edit_visible) {
                          this.submitSearch();
                      }
                      return false;
                  }}>
                  <InputGroup className="mb-3">
                  <Button
                    size="sm"
                    type="button"
                    variant="outline-dark"
                    className={classname}
                    onClick={()=>{
                        if (this.state.edit_visible) {
                            this.submitSearch();
                            return;
                        }

                        // Set the focus on the element outside the react loop.
                        window.setTimeout(()=>{
                            const el = document.getElementById(this.guid());
                            if (el) {
                                el.focus();
                            }}, 0);

                        this.setState({
                            edit_visible: true,
                        });
                        this.props.setTransform({editing: this.props.column});
                    }}>
                    <FontAwesomeIcon icon="filter"/>
                  </Button>
                      <Form.Control
                        as="input"
                        id={this.guid()}
                        placeholder={T("Regex Filter")}
                        spellCheck="false"
                        className={classname}
                        value={this.state.edit_filter}
                        onChange={e=> {
                          this.setState({edit_filter: e.currentTarget.value});
                      }}/>
                    </InputGroup>
                </Form>
              </ToolTip>
            );
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


        return (
            <ToolTip tooltip={T("Sort")}>
              <Button
                size="sm"
                className={classname}
                variant="outline-dark"
                onClick={()=>this.submitSort(next_dir)}
              >
                <FontAwesomeIcon icon={icon}/>
              </Button>
            </ToolTip>
        );
    }
}


export class ColumnToggle extends Component {
    static propTypes = {
        columns: PropTypes.array,
        toggles: PropTypes.object,
        onToggle: PropTypes.func,
        swapColumns: PropTypes.func,
        headers: PropTypes.object,
    }

    state = {
        opened: false,
    }

    render() {
        let enabled_columns = [];
        let buttons = _.map(this.props.columns, (column, idx) => {
            if (!column) {
                return <React.Fragment key={idx}></React.Fragment>;
            }
            let hidden = this.props.toggles[column];
            if (!hidden) {
                enabled_columns.push(column);
            }
            let header = this.props.headers && this.props.headers[column];
            if (!_.isString(header)) {
                header = column;
            }
            return <Dropdown.Item
                     key={ column }
                     eventKey={column}
                     className="draggable"
                     active={!hidden}
                     draggable="true"
                     onDragStart={e=>{
                         e.dataTransfer.setData("column", column);
                     }}
                     onDrop={e=>{
                         e.preventDefault();
                         this.props.swapColumns(
                             e.dataTransfer.getData("column"), column);
                     }}
                     onDragOver={e=>{
                         e.preventDefault();
                         e.dataTransfer.dropEffect = "move";
                     }}
                   >
                     { header }
            </Dropdown.Item>;
        });

        return <ToolTip tooltip={T("Show/Hide Columns")}>
                 <Dropdown as={ButtonGroup}
                           show={this.state.open}
                           onSelect={this.props.onToggle}
                           onToggle={(nextOpen, metadata) => {
                               this.setState({
                                   open: metadata.source === "select" || nextOpen,
                               });
                           }}>
                   <Dropdown.Toggle variant="default" id="dropdown-basic">
                     <FontAwesomeIcon icon="columns"/>
                   </Dropdown.Toggle>

                   <Dropdown.Menu>
                     { _.isEmpty(enabled_columns) ?
                       <Dropdown.Item
                         onClick={()=>{
                             // Enable all columns
                             _.each(this.props.columns, this.props.onToggle);
                         }}>
                         {T("Set All")}
                       </Dropdown.Item> :
                       <Dropdown.Item
                         onClick={()=>{
                             // Disable all enabled columns
                             _.each(enabled_columns, this.props.onToggle);
                         }}>
                         {T("Clear All")}
                       </Dropdown.Item> }
                     <Dropdown.Divider />
                     { buttons }
                   </Dropdown.Menu>
                 </Dropdown>
               </ToolTip>;
    };
}


export class TablePaginationControl extends React.Component {
    static propTypes = {
        page_size:  PropTypes.number,
        total_size: PropTypes.number,
        current_page: PropTypes.number,
        start_row: PropTypes.number,
        onRowChange: PropTypes.func,
        onPageSizeChange: PropTypes.func,
        direction: PropTypes.string,
    }

    state = {
        goto_offset: "",
        goto_error: false,
    }

    componentDidUpdate(prevProps, prevState, snapshot) {
        // The current cursor points beyond the end of the table, seek
        // to the last page. This can happen if the table suddenly
        // shrinks.
        if (this.props.start_row > this.props.total_size) {
            let last_page = this.getLastPage();
            let last_row = last_page * this.props.page_size;
            if(last_row !== this.props.start_row) {
                this.props.onRowChange(last_row);
            }
        }
    }

    renderLabel = (start, end, total_size)=>{
        end = end || 0;
        start = start || 0;
        total_size = total_size || 0;
        let total_size_len = total_size.toString().length;
        let total_length = total_size_len * 3 + 2;
        let start_row_len = start.toString().length;
        let end_len = end.toString().length;
        let padding_len = total_length - total_size_len -
            start_row_len - end_len - 2;
        let padding =  [];
        for(let i = 0; i<padding_len;i++) {
            padding.push(<span key={i}>&nbsp;</span>);
        };

        return <>{padding}{start}-{end}/{total_size}</>;
    }

    getLastPage = ()=>{
        let total_size = parseInt(this.props.total_size || 0);
        let total_pages = parseInt(total_size / this.props.page_size) + 1;
        let last_page = total_pages - 1;

        // Ensure the last page has some data - otherwise back up one
        // page.
        if (last_page * this.props.page_size===this.props.total_size) {
            last_page -= 1;
        }

        if (last_page <= 0) {
            last_page = 0;
        }

        return last_page;
    }

    render() {
        let last_page = this.getLastPage();
        let pages = [];
        let current_page = parseInt(this.props.start_row / this.props.page_size);
        let start_page = current_page - 4;
        let end_page = current_page + 4;

        let end = this.props.start_row + this.props.page_size;
        if(end>this.props.total_size) {
            end=this.props.total_size;
        }

        if (start_page < 0) {
            end_page -= start_page;
        }

        if (end_page > last_page) {
            start_page -= end_page - last_page;
        }

        if (start_page < 0 ) {
            start_page = 0;
        }

        if( end_page > last_page) {
            end_page = last_page;
        }

        for(let i=start_page; i<end_page + 1; i++) {
            let row_offset = i * this.props.page_size;
            pages.push(
                <Dropdown.Item as={Button}
                               variant="default"
                               key={i}
                               active={i === current_page}
                               onClick={ () => this.props.onRowChange(row_offset)} >
                  { row_offset }
                </Dropdown.Item>);
        };

        let page_sizes = _.map([10, 25, 30, 50, 100], x=>{
            return <Dropdown.Item
                     as={Button}
                     variant="default"
                     key={x}
                     active={x === this.props.page_size}
                     onClick={ () => this.props.onPageSizeChange(x) } >
                     { x }
                   </Dropdown.Item>;
        });

        return (
            <ButtonGroup>
              <Button variant="default" className="goto-start"
                      disabled={current_page===0}
                      onClick={()=>this.props.onRowChange(0)}>
                <FontAwesomeIcon icon="backward-fast"/>
              </Button>

              <Button variant="default" className="goto-prev"
                      disabled={current_page===0}
                      onClick={()=>this.props.onRowChange(
                          (current_page-1) * this.props.page_size)}>
                <FontAwesomeIcon icon="backward"/>
              </Button>

              <ToolTip tooltip={T("Goto Page")}>
                <Dropdown as={ButtonGroup}
                          show={this.state.open}
                          drop={this.props.direction}
                          onSelect={this.selectPage}
                          onToggle={(nextOpen, metadata) => {
                              this.setState({
                                  open: metadata.source === "select" || nextOpen,
                              });
                          }}>
                  <Dropdown.Toggle variant="default" id="dropdown-basic">
                    {this.renderLabel(this.props.start_row,
                                      end, this.props.total_size)}
                  </Dropdown.Toggle>

                  <Dropdown.Menu>
                    <Dropdown.Item className="goto-input">
                      <Form.Control
                        as="input"
                        className="pagination-form"
                        placeholder={T("Goto Page")}
                        spellCheck="false"
                        id="goto-page"
                        value={this.props.start_row || ""}
                        onChange={e=> {
                            let new_row = parseInt(e.currentTarget.value || 0);
                            if (new_row >= 0 && new_row < this.props.total_size) {
                                this.props.onRowChange(new_row);
                            }
                        }}/>
                    </Dropdown.Item>
                    <Dropdown.Divider />
                    { pages }
                  </Dropdown.Menu>
                </Dropdown>
              </ToolTip>

              <Button variant="default" className="goto-next"
                      disabled={this.props.current_page >= last_page}
                      onClick={()=>this.props.onRowChange(
                          (current_page+1) * this.props.page_size)}>
                <FontAwesomeIcon icon="forward"/>
              </Button>

              <Button variant="default" className="goto-end"
                      disabled={this.props.current_page === last_page}
                      onClick={()=>this.props.onRowChange(
                          last_page * this.props.page_size)}>
                    <FontAwesomeIcon icon="forward-fast"/>
              </Button>

              <ToolTip tooltip={T("Page Size")}>
                <Dropdown as={ButtonGroup}
                          show={this.state.open_size}
                          drop={this.props.direction}
                          onSelect={this.selectPage}
                          onToggle={(nextOpen, metadata) => {
                              this.setState({
                                  open_size: metadata.source === "select" || nextOpen,
                              });
                          }}>
                  <Dropdown.Toggle variant="default" id="dropdown-basic">
                    {this.props.page_size || 0}
                  </Dropdown.Toggle>

                  <Dropdown.Menu>
                    { page_sizes }
                  </Dropdown.Menu>
                </Dropdown>
              </ToolTip>
            </ButtonGroup>
        );
    }
}


class VeloPagedTable extends Component {
    static contextType = UserConfig;

    static propTypes = {
        // Params to the GetTable API call.
        params: PropTypes.object,

        // A dict containing renderers for each column.
        renderers: PropTypes.object,

        // An alternative to renderers - resolved using getFormatter()
        formatters: PropTypes.object,

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

        // If set we update the callback with the pagination state.
        setPageState: PropTypes.func,

        // React router props.
        is_fullscreen: PropTypes.bool,
        history: PropTypes.object,
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

        // Map between columns and their widths.
        column_widths: {},

        compact_columns: {},

        start_selection_idx: -1,

        guid: "",
    }

    componentDidMount = () => {
        this.source = CancelToken.source();
        this.setState({
            guid: getID(),
            page_size: this.props.initial_page_size || 10});

        this.fetchRows();
    }

    componentWillUnmount() {
        this.source.cancel();
    }

    componentDidUpdate(prevProps, prevState, snapshot) {
        // If the table has changed we need to clear the state
        // completely as it is a new table.
        if (!_.isUndefined(this.props.name) &&
            !_.isEqual(this.props.name, prevProps.name)) {
            this.setState({toggles: {}, rows: [], columns: []});
        }

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
        let row_data = {};
        _.each(this.activeColumns(), x=>{
            row_data[x] = row[x];
        });
        return <VeloValueRenderer value={cell} row={row_data} />;
    }

    getColumnRenderer = column => {
        if(this.props.formatters && this.props.formatters[column]) {
            return getFormatter(this.props.formatters[column], column);
        }

        if(this.props.renderers && this.props.renderers[column]) {
            return this.props.renderers[column];
        }

        let column_types = this.state.column_types;
        if (!_.isArray(column_types)) {
            return this.defaultFormatter;
        }

        for (let i=0; i<column_types.length; i++) {
            if (column === column_types[i].name) {
                return getFormatter(column_types[i].type, column_types[i].name);
            }
        }
        return this.defaultFormatter;
    }

    // Render the transformed view at the top right of the
    // table. Shows the user what transforms are currnetly active.
    getTransformed = ()=>{
        let transform = Object.assign({}, this.state.transform || {});
        if(_.isEmpty(transform) && !_.isEmpty(this.props.transform)) {
            Object.assign(transform, this.props.transform);
        }
        return transform;
    }

    getTransformedRenderer = (transform)=>{
        if(_.isEmpty(transform)) {
            return <></>;
        }
        return <TransformViewer
                 transform={transform}
                 setTransform={t=>this.setState({transform: t})}
               />;
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
                           columns: this.mergeColumns(columns) });

            if(this.props.setPageState) {
                this.props.setPageState({
                    total_size: parseInt(response.data.total_rows || 0),
                    start_row: this.state.start_row,
                    page_size: this.state.page_size,
                    onRowChange: row_offset=>this.setState({start_row: row_offset}),
                    onPageSizeChange: size=>this.setState({page_size: size}),
                });
            }

            if(this.state.select_on_load >= 0 &&
               this.state.select_on_load < pageData.rows.length) {
                let row = pageData.rows[this.state.select_on_load];
                this.selectRow(row, this.state.select_on_load, false);
                this.setState({select_on_load: -1});
            }

        }).catch(response=>{
            this.setState({loading: false, rows: [], columns: [], stack_path: []});
        });
    }

    // Merge new columns into the current table state in such a way
    // that the existing column ordering will not be changed.
    mergeColumns = columns=>{
        let lookup = {};
        _.each(columns, x=>{
            lookup[x] = true;
        });
        let new_columns = [];
        let new_lookup = {};

        // Add the old columns only if they are also in the new set,
        // preserving their order.
        _.each(this.state.columns, c=>{
            if(lookup[c]) {
                new_columns.push(c);
                new_lookup[c] = true;
            }
        });

        // Add new columns if they were not already, preserving their
        // order.
        _.each(columns, c=>{
            if(!new_lookup[c])  {
                new_columns.push(c);
            }
        });

        return new_columns;
    }

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

    activeColumns = ()=>{
        let res = [];
        _.each(this.state.columns, c=>{
            if(!this.state.toggles[c]) {
                res.push(c);
            }
        });
        return res;
    }

    fullscreen_state = ()=>{
        let state = {
            params: this.props.params,
            url: this.props.url,
            env: this.props.env,
            formatters: this.props.formatters,
        };
        return "/fullscreen/table/" + btoa(serializeJSON(state));
    }

    renderToolbar = ()=>{
        if(this.props.no_toolbar) {
            return <></>;
        };

        let timezone = (this.context.traits &&
                        this.context.traits.timezone) || "UTC";

        let transformed = this.getTransformed();
        let active_columns = this.activeColumns();
        let downloads = Object.assign({}, this.props.params);

        if (!_.isEqual(this.state.all_columns, active_columns)) {
            downloads.columns = active_columns;
        }

        if(transformed.filter_column) {
            downloads.filter_column = transformed.filter_column;
            downloads.filter_regex = transformed.filter_regex;
        }

        if (transformed.sort_column) {
            downloads.sort_column = transformed.sort_column;
            downloads.sort_direction = transformed.sort_direction === "Ascending";
        }

        let all_compacted = true;
        let none_compacted = true;

        _.each(this.state.compact_columns, (v, k)=>{
            if (v === true) {
                none_compacted = false;
            } else {
                all_compacted = false;
            };
        });

        return (
            <Navbar className="toolbar">
              <ButtonGroup>
                <ToolTip tooltip={!all_compacted ?
                                  T("Collapse all columns") :
                                  T("Expand all columns")}>
                  <Button variant="default"
                          onClick={()=>{
                              // Compact all columns if any columns are expanded.
                              // If they are all compacted then expand them all.
                              let compact = none_compacted || !all_compacted;
                              let compact_columns = {};
                              _.each(this.state.columns, c=>{
                                  compact_columns[c] = compact;
                              });
                              this.setState({compact_columns: compact_columns});
                    }}
                  >
                    <FontAwesomeIcon icon={!all_compacted ? "compress": "expand"}/>
                  </Button>
                </ToolTip>
                <ColumnToggle onToggle={(c)=>{
                    // Do not make a copy here because set state is
                    // not immediately visible and this will be called
                    // for each column.
                    let toggles = this.state.toggles;
                    toggles[c] = !toggles[c];
                    this.setState({toggles: toggles});
                }}
                              columns={this.state.columns}
                              swapColumns={this.swapColumns}
                              toggles={this.state.toggles} />
                <InspectRawJson rows={this.state.rows} />
                <ToolTip tooltip={T("Download JSON")}>
                  <Button variant="default"
                          target="_blank" rel="noopener noreferrer"
                          href={api.href("/api/v1/DownloadTable",
                                         Object.assign(downloads, {
                                             timezone: timezone,
                                             download_format: "json",
                                         }))}>
                    <FontAwesomeIcon icon="download"/>
                    <span className="sr-only">{T("Download JSON")}</span>
                  </Button>
                </ToolTip>
                <ToolTip tooltip={T("Download CSV")}>
                  <Button variant="default"
                          target="_blank" rel="noopener noreferrer"
                          href={api.href("/api/v1/DownloadTable",
                                         Object.assign(downloads, {
                                             timezone: timezone,
                                             download_format: "csv",
                                         }))}>
                    <FontAwesomeIcon icon="file-csv"/>
                    <span className="sr-only">{T("Download CSV")}</span>
                  </Button>
                </ToolTip>
                { !this.props.is_fullscreen &&
                  <ToolTip tooltip={T("Fullscreen")}>
                    <Link to={this.fullscreen_state()}
                          target="_blank" rel="noopener noreferrer"
                          role="button" className="btn btn-default">
                      <FontAwesomeIcon icon="maximize"/>
                      <span className="sr-only">{T("Fullscreen")}</span>
                    </Link>
                  </ToolTip> }

              </ButtonGroup>
              { this.renderPaginator() }
              <ButtonGroup className="float-right">
                { this.getTransformedRenderer(transformed) }
              </ButtonGroup>
              { this.props.toolbar || <></> }
            </Navbar>
        );
    }

    renderHeader = (column, idx)=>{
        // Derive the name of the column to put in the header.
        // It can be set by specifying the `text` attribute
        // of the columns prop.
        let extra_columns = this.props.columns;
        let column_name = (
            extra_columns &&
                extra_columns[column] &&
                extra_columns[column].text) || column;
        if (this.props.translate_column_headers) {
            column_name = T(column_name);
        }

        let styles = {};
        let col_width = this.state.column_widths[column_name];
        if (col_width) {
            styles = {
                minWidth: col_width,
                maxWidth: col_width,
                width: col_width,
            };
        }

        // Only show those icons on columns which have transformations
        // applied on them.
        let transformed_class = "";
        if (this.isColumnStacked(column)) {
            transformed_class = "transformed";
        }

        let t_desc = this.getDesc(column_name);

        // Do not allow this column to be sorted/filtered
        if (this.props.prevent_transformations &&
            this.props.prevent_transformations[column]) {
            return <React.Fragment key={idx}>
                     <th style={styles}
                         className={transformed_class + " draggable paged-table-header"}
                         onDragStart={e=>{
                             e.dataTransfer.setData("column", column);
                         }}
                         onDrop={e=>{
                             e.preventDefault();
                             this.swapColumns(
                                 e.dataTransfer.getData("column"), column);
                         }}
                         onDragOver={e=>{
                             e.preventDefault();
                             e.dataTransfer.dropEffect = "move";
                         }}
                         draggable="true">
                       <span className="column-name">
                         { t_desc ?
                           <ToolTip tooltip={t_desc}>
                             { t_desc }
                           </ToolTip> : column_name }
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
                   </React.Fragment>;
        }

        return (
            <React.Fragment key={idx}>
              <th style={styles}
                  className={transformed_class + " draggable paged-table-header"}
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
                  { t_desc ?
                    <ToolTip tooltip={t_desc}>
                      <span>{ column_name }</span>
                    </ToolTip> : column_name }
                </span>
                <span className="sort-element">
                  { this.state.transform.editing ?
                    <ButtonGroup className="hover-buttons">
                      <ColumnFilter column={column}
                                    table_id={this.state.guid}
                                    transform={this.state.transform}
                                    setTransform={this.setTransform}
                      />
                    </ButtonGroup>
                    :
                    <ButtonGroup className="hover-buttons">
                      { this.isColumnStacked(column) &&
                        <ToolTip tooltip={T("Stack")} key={idx}>
                          <Button variant="default"
                                  className="visible"
                                  target="_blank" rel="noopener noreferrer"
                                  onClick={e=>{
                                      this.setState({showStackDialog: column});
                                      e.preventDefault();
                                      return false;
                                  }}>
                            <FontAwesomeIcon icon="layer-group"/>
                          </Button>
                        </ToolTip>
                      }
                      <ColumnSort column={column}
                                  transform={this.state.transform}
                                  setTransform={this.setTransform}
                      />

                      <ColumnFilter column={column}
                                    table_id={this.state.guid}
                                    transform={this.state.transform}
                                    setTransform={this.setTransform}
                      />
                      <Button
                        size="sm"
                        type="button"
                        variant="outline-dark"
                        className="hidden-edit"
                        onClick={()=>{
                            let compact_columns = Object.assign(
                                {}, this.state.compact_columns);
                            compact_columns[column] = !compact_columns[column];
                            this.setState({
                                compact_columns: compact_columns,
                            });
                        }}>
                        { !this.state.compact_columns[column] ?
                          <ToolTip tooltip={T("Compact Column")}>
                            <FontAwesomeIcon icon="compress"/>
                          </ToolTip> :
                          <ToolTip tooltip={T("Expand Column")}>
                            <FontAwesomeIcon icon="expand"/>
                          </ToolTip>
                        }
                      </Button>
                    </ButtonGroup>
            }
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

    isCellCollapsed = (column, rowIdx) => {
        let column_desc = this.state.compact_columns[column];
        // True represents all the cells are collapsed
        if (column_desc === true) {
            return true;
        }
        // If we store an object here then the object represents only
        // cells which are **not** collapsed.
        if(_.isObject(column_desc)) {
            if(column_desc[rowIdx.toString()]) {
                return false;
            }
            return true;
        };
        return false;
    }

    renderCell = (column, row, rowIdx) => {
        let t = this.state.toggles[column];
        if(t) {return undefined;};

        let cell = row[column];
        let renderer = this.getColumnRenderer(column);
        let is_collapsed = this.isCellCollapsed(column, rowIdx);
        let clsname = is_collapsed ? "compact": "";

        return <React.Fragment key={column}>
                 <td className={clsname}
                     onClick={()=>{
                         // If the column is not collapsed no click handler!
                         if(!is_collapsed) return;

                         // The cell is collapsed, we need to expand it:

                         // If the entire column is collapsed we need
                         // to specify that only this row is expanded.
                         let column_desc = this.state.compact_columns[column];
                         if(column_desc === true) {
                             column_desc = {};
                         }
                         column_desc[rowIdx.toString()]=true;
                         let compact_columns = Object.assign(
                             {}, this.state.compact_columns);
                         compact_columns[column] = column_desc;
                         this.setState({compact_columns: compact_columns});
                     }}
                   >
                   { renderer(cell, row, this.props.env)}
                 </td>
                 <ColumnResizer
                   className={clsname}
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

    selectRow = (row, idx, start_region)=>{
        if (!this.props.selectRow) {
            return;
        }

        if(this.props.selectRow.onSelect) {
            this.props.selectRow.onSelect(row);
        }

        if(this.props.selectRow.onMultiSelect) {
            if(start_region || this.state.start_selection_idx < 0) {
                this.props.selectRow.onMultiSelect([row]);
            } else {
                let rows = [];
                let first = this.state.start_selection_idx;
                let last = idx;
                if (first > last) {
                    let tmp = last;
                    last = first;
                    first = tmp;
                }

                for(let i=first;i<=last;i++) {
                    rows.push(this.state.rows[i]);
                };

                this.props.selectRow.onMultiSelect(rows);
            }

            // First click without shift: Clear end selection and set start selection.
            if(start_region) {
                this.setState({end_selection_idx: -1, start_selection_idx: idx});

                // Second click with a shift: Set end selection
            } else if (this.state.start_selection_idx >= 0) {
                this.setState({end_selection_idx: idx});
            }
        }

        this.setState({selected_row: row, selected_row_idx: idx});
    }

    isRowSelected = idx=>{
        if(this.state.start_selection_idx >= 0 &&
           this.state.end_selection_idx >= 0) {
            let start = this.state.start_selection_idx;
            let end = this.state.end_selection_idx;
            if (start > end) {
                let tmp = start;
                start = end;
                end = tmp;
            };
            return idx >= start  && idx <= end;
        }
        return this.state.selected_row_idx === idx;
    }

    renderRow = (row, idx)=>{
        let last_selected_cls = (this.props.selectRow &&
                            this.props.selectRow.classes) || "row-selected";
        let selected_cls = "";
        if(this.state.selected_row_idx === idx) {
            selected_cls += " " + last_selected_cls;
        } else if(this.isRowSelected(idx)) {
            selected_cls += " row-multi-selected " + last_selected_cls;
        }

        if(this.props.row_classes) {
            selected_cls += " " + this.props.row_classes(row, idx);
        }

        return (
            <tr key={idx}
                onClick={e=>{
                    // Prevent the browser from selecting with shift key
                    if (e.shiftKey) {
                        document.getSelection().removeAllRanges();
                    }
                    this.selectRow(row, idx, !e.shiftKey);
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

    getLastPage = ()=>{
        let total_size = parseInt(this.state.total_size || 0);
        let total_pages = parseInt(total_size / this.state.page_size) + 1;
        let last_page = total_pages - 1;

        // Ensure the last page has some data - otherwise back up one
        // page.
        if (last_page * this.state.page_size===this.state.total_size) {
            last_page -= 1;
        }

        if (last_page <= 0) {
            last_page = 0;
        }

        return last_page;
    }

    nextSelection = (e)=>{
        if(e){
            e.preventDefault();
            e.stopPropagation();
        }


        let rows_length = this.state.rows.length;
        if(rows_length === 0 || this.state.selected_row_idx < 0 ||
           !this.props.selectRow) {
            return;
        };

        let next_row = this.state.selected_row_idx + 1;
        if(next_row < rows_length) {
            let row = this.state.rows[next_row];
            this.selectRow(row, next_row, true);
        } else if(next_row + this.state.start_row < this.state.total_size) {
            // Next page
            this.nextPage();
            this.setState({select_on_load: 0});
        }
    }

    prevSelection = e=>{
        if(e){
            e.preventDefault();
            e.stopPropagation();
        }

        if(this.state.selected_row_idx >= 0 && this.props.selectRow) {
            let prev_row = this.state.selected_row_idx - 1;

            if(prev_row >= 0) {
                let row = this.state.rows[prev_row];
                this.selectRow(row, prev_row, true);
            } else if(prev_row + this.state.start_row >= 0) {
                // Previous page
                this.previousPage();
                this.setState({select_on_load: 0});
            }
        }
    }

    nextPage = (e)=>{
        if(e){
            e.preventDefault();
            e.stopPropagation();
        }
        let last_page = this.getLastPage();
        let current_page = parseInt(this.state.start_row / this.state.page_size);
        let next_page = current_page + 1;

        if ( next_page <= last_page) {
            this.setState({start_row: next_page * this.state.page_size });
        }

        if(this.state.selected_row_idx >= 0 && this.props.selectRow) {
            this.setState({select_on_load: this.state.selected_row_idx});
        }
    }

    previousPage = (e)=>{
        if(e) {
            e.preventDefault();
            e.stopPropagation();
        }

        let current_page = parseInt(this.state.start_row / this.state.page_size);
        let next_page = current_page - 1;
        if ( next_page >= 0) {
            this.setState({start_row: next_page * this.state.page_size });
        }

        if(this.state.selected_row_idx >= 0 && this.props.selectRow) {
            this.setState({select_on_load: this.state.selected_row_idx});
        }
    }

    gotoHome = e=>{
        if(e) {
            e.preventDefault();
            e.stopPropagation();
        }

        this.setState({start_row: 0 });
        if(this.state.selected_row_idx >= 0 && this.props.selectRow) {
            this.setState({select_on_load: 0});
        }
    }

    gotoEnd = e=>{
        if(e) {
            e.preventDefault();
            e.stopPropagation();
        }

        let last_page = this.getLastPage();
        this.setState({start_row: last_page * this.state.page_size });

        if(this.state.selected_row_idx >= 0 && this.props.selectRow) {
            this.setState({select_on_load: 0});
        }
    }

    renderPaginator = (direction)=>{
        let end = this.state.start_row + this.state.page_size;
        if(end>this.state.total_size) {
            end=this.state.total_size;
        }

        return (
            <>
              <HotKeys keyMap={this.keymap} component={"span"} handlers={this.handlers}>
                <TablePaginationControl
                  total_size={this.state.total_size}
                  start_row={this.state.start_row}
                  page_size={this.state.page_size}
                  current_page={this.state.start_row / this.state.page_size}
                  onRowChange={row_offset=>this.setState({start_row: row_offset})}
                  onPageSizeChange={size=>this.setState({page_size: size})}
                  direction={direction}
                />
              </HotKeys>
            </>
        );
    }

    getDesc = c=>{
        let types = this.state.column_types || [];
        for(let i=0;i<types.length;i++) {
            if(types[i].name === c) {
                return types[i].description;
            }
        }
        return undefined;
    }

    renderTable = ()=>{
        return (
            <>
              <HotKeys keyMap={this.keymap}
                       className="table-panel"
                       component={"div"} handlers={this.handlers}>
                <Table tabIndex="0" className="paged-table">
                  <thead>
                    <tr className="paged-table-header">
                      {_.map(this.activeColumns(), this.renderHeader)}
                    </tr>
                  </thead>
                  <tbody className="fixed-table-body">
                    {_.map(this.state.rows, this.renderRow)}
                  </tbody>
                </Table>
              </HotKeys>

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
            </>
        );
    }

    keymap={
        NEXT: "n",
        PREVIOUS: "p",
        NEXT_SELECTION: "k",
        PREVIOUS_SELECTION: "j",
        END: "End",
        HOME: "Home",
    };

    handlers={
        NEXT: this.nextPage,
        PREVIOUS: this.previousPage,
        NEXT_SELECTION: this.nextSelection,
        PREVIOUS_SELECTION: this.prevSelection,
        END: this.gotoEnd,
        HOME: this.gotoHome,
    };

    render = ()=>{
        return (
            <>
                  { this.renderToolbar() }
                  { _.isEmpty(this.state.columns) ?
                    <div className="no-content">
                      {T("Table contains no data")}
                    </div>
                    : this.renderTable()}
            </>);
    }
}

export default VeloPagedTable;


export class TransformViewer extends Component {
    static propTypes = {
        transform: PropTypes.object,
        setTransform: PropTypes.func,
    }

    render() {
        let transform = this.props.transform || {};
        let result = [];

        if (transform.filter_column) {
            result.push(
                <div className="transform-viewer"  key={result.length}>
                  <ToolTip tooltip={T("Clear")} key="1">
                    <Button onClick={()=>{
                        let new_transform = Object.assign({}, this.props.transform);
                        new_transform.filter_column = undefined;
                        this.props.setTransform(new_transform);
                    }}
                            className="table-transformed"
                            variant="outline-dark">
                      { transform.filter_column } ( {transform.filter_regex} )
                      <span className="transform-button">
                        <FontAwesomeIcon icon="filter"/>
                      </span>
                    </Button>
                  </ToolTip>
                </div>
            );
        }

        if (transform.sort_column) {
            result.push(
                <div className="transform-viewer" key={result.length}>
                  <ToolTip tooltip={T("Transformed")} key="2" >
                    <Button onClick={()=>{
                        let new_transform = Object.assign({}, this.props.transform);
                        new_transform.sort_column = undefined;
                        this.props.setTransform(new_transform);
                    }}
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
                  </ToolTip>
                </div>
            );
        }

        return <>{result}</>;
    }
}
