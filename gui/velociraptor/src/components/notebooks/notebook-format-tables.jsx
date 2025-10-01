import _ from 'lodash';

import React, { Component } from 'react';
import PropTypes from 'prop-types';
import Modal from 'react-bootstrap/Modal';
import T from '../i8n/i8n.jsx';
import Button from 'react-bootstrap/Button';
import Form from 'react-bootstrap/Form';
import Row from 'react-bootstrap/Row';
import Col from 'react-bootstrap/Col';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { JSONparse } from '../utils/json_parse.jsx';

// components/core/table.jsx getFormatter
const column_types = [
    "string", "number", "mb", "timestamp", "nobreak",
    "json/0", "json/1", "json/2", "json/3",
    "url", "url_internal", "safe_url", "flow", "preview_upload",
    "download", "client", "hex", "base64", "collapsed"
];

const column_type_regex = /^([\s\S]*)LET ColumnTypes<=dict\((.+)\)\n\n([\S\s]+)$/m;
const column_type_args_regex = /`([^`]+)`='([^']+)'/g;

export default class FormatTableDialog extends Component {
    static propTypes = {
        cell: PropTypes.object,
        notebook_metadata: PropTypes.object,
        saveCell: PropTypes.func,
        closeDialog: PropTypes.func,
        columns: PropTypes.array,
    }

    state = {
        selection: [],
        selected_columns: [],
        new_selection_column: "",
        new_selection_type: "string",
        prefix: "",
        suffix: "",
    }

    componentDidMount() {
        let current_input = this.props.cell && this.props.cell.input;
        this.parseColumnTypes(current_input);
    }

    componentDidUpdate = (prevProps, prevState, rootNode) => {
        let last_input = prevProps.cell && prevProps.cell.input;
        let current_input = this.props.cell && this.props.cell.input;
        if (last_input !== current_input) {
            this.parseColumnTypes(current_input);
        }
    }

    addColumn = () => {
        let selection = this.state.selection;
        selection.push({column: this.state.new_selection_column,
                        type: this.state.new_selection_type});
        this.setState({selection: selection,
                       new_selection_column: "",
                       selected_columns: _.map(selection, x=>x.column),
                       new_selection_type: "string"});
    }

    removeColumn = column=>{
        let selection = _.filter(this.state.selection || [],
                                 x=>x.column !== column);
        this.setState({selection: selection,
                       selected_columns: _.map(selection, x=>x.column)});
    }

    // Get the column types from the notebook env. We use that as the
    // initial set.
    getDefaultColumnTypes = ()=>{
        let res = {};
        let md = this.props.notebook_metadata || {};
        let requests = md.requests || [];
        _.each(requests, x=>{
            _.each(x.env, x=>{
                if(x.key === "ColumnTypes") {
                    _.each(JSONparse(x.value), (v, k)=>{
                        res[k] = v;
                    });
                }
            });
        });

        return res;
    }

    parseColumnTypes = vql=>{
        const default_types = this.getDefaultColumnTypes();
        let selection = [];
        let selected_columns = [];
        let types = {};
        let prefix = "";
        let suffix = "";
        let match = vql && vql.match(column_type_regex);
        if(match && match.length > 0) {
            prefix = match[1];
            suffix = match[3];

            let args = match[2];
            let arg_match = [...args.matchAll(column_type_args_regex)];
            _.each(arg_match, x=>{
                selected_columns.push(x[1]);
                selection.push({column: x[1], type: x[2]});
                types[x[1]] = x[2];
            });
        } else {
            suffix = vql;
        }

        // merge the default types with the selected types.
        _.each(default_types, (v, k)=>{
            if(!types[k]) {
                selection.push({column: k, type: v});
                selected_columns.push(k);
            };
        });

        this.setState({selection: selection,
                       selected_columns: selected_columns,
                       new_selection_column: "",
                       prefix: prefix,
                       suffix: suffix,
                       new_selection_type: "string"});
    }

    updateCell = () => {
        let selection = this.state.selection || [];

        if(this.state.new_selection_column) {
            selection.push({column: this.state.new_selection_column,
                            type: this.state.new_selection_type});
        }

        if (!_.isEmpty(selection)) {
            // Build the VQL for ColumnTypes
            let vql = "LET ColumnTypes<=dict("+
                _.map(this.state.selection,
                      x=>"`"+x.column+"`='"+x.type+"'").join(",") + ")\n\n";
            let cell = this.props.cell;
            cell.input = this.state.prefix + vql + this.state.suffix;

            this.props.saveCell(cell);
        }
        this.props.closeDialog();
    }

    render() {
        return (
            <Modal show={true}
                   size="lg"
                   className="full-height"
                   dialogClassName="modal-90w"
                   onHide={this.props.closeDialog} >
              <Modal.Header closeButton>
                <Modal.Title>{T("Change column types")}</Modal.Title>
              </Modal.Header>
              <Modal.Body>
                <h3>{T("Update the table column types in this cell.")}</h3>
                {_.map(this.state.selection || [], (x, idx)=>{
                    return <Row key={idx}>
                             <Col sm="4">{x.column}</Col>
                             <Col sm="4">{x.type}</Col>
                             <Col sm="4">
                               <Button  variant="default-outline"
                                        onClick={()=>this.removeColumn(x.column)}
                                        size="sm">
                                 <FontAwesomeIcon icon="minus"/>
                               </Button>
                             </Col>
                           </Row>;
                })}
                <hr/>
                <Row>
                  <Col sm="4">
                    <Form.Control as="select"
                                  value={this.state.new_selection_column}
                                  onChange={(e) => {
                                      this.setState({new_selection_column: e.currentTarget.value});
                                  }}>
                      <option key='b' hidden value>
                        {T("Select Column")}
                      </option>
                      {_.map(_.difference(this.props.columns || [],
                                          this.state.selected_columns),
                             function(item, idx) {
                                 return <option key={idx}>{item}</option>;
                             })}
                    </Form.Control>
                  </Col>
                  <Col sm="4">
                    <Form.Control as="select"
                                  value={this.state.new_selection_type}
                                  onChange={(e) => {
                                      this.setState({new_selection_type: e.currentTarget.value});
                                  }}>
                      {_.map(column_types || [], function(item, idx) {
                          return <option key={idx}>{item}</option>;
                      })}
                    </Form.Control>
                  </Col>
                  <Col sm="4">
                    <Button  variant="default-outline"
                      disabled={!this.state.new_selection_column ||
                                !this.state.new_selection_type}
                             onClick={this.addColumn}
                             size="sm">
                      <FontAwesomeIcon icon="plus"/>
                    </Button>
                  </Col>
              </Row>
              </Modal.Body>
              <Modal.Footer>
                <Button variant="secondary"
                        onClick={this.props.closeDialog}>
                  {T("Cancel")}
                </Button>
                <Button variant="primary"
                        onClick={this.updateCell}>
                  {T("Submit")}
                </Button>
              </Modal.Footer>
            </Modal>
        );
    }
}
