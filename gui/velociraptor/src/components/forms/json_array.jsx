import _ from 'lodash';

import T from '../i8n/i8n.jsx';
import PropTypes from 'prop-types';
import ButtonGroup from 'react-bootstrap/ButtonGroup';
import Button from 'react-bootstrap/Button';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import Form from 'react-bootstrap/Form';
import Row from 'react-bootstrap/Row';
import React, { Component } from 'react';
import Col from 'react-bootstrap/Col';
import Alert from 'react-bootstrap/Alert';
import ToolTip from '../widgets/tooltip.jsx';
import Table from 'react-bootstrap/Table';
import { ValidateJSON, JSONparse,
         serializeJSON } from "../utils/json_parse.jsx";


export default class JSONArrayForm extends Component {
    static propTypes = {
        param: PropTypes.object,
        value: PropTypes.string,
        setValue: PropTypes.func.isRequired,
    };

    state = {
        mode: "json",
        error: "",
    }

    setJSONValue = value=>{
        // Check if we can parse it properly.
        this.setState({error: !ValidateJSON(value)});
        this.props.setValue(value);
    }

    render() {
        if (this.state.mode === "json") {
            return this.renderJSONTable();
        }
        return this.renderTextArea();
    }

    renderTextArea() {
        let param = this.props.param || {};
        let name = param.friendly_name || param.name;

        return (
            <Form.Group as={Row}>
              <Form.Label column sm="3">
                <ToolTip tooltip={param.description}>
                  <div>
                    {name}
                  </div>
                </ToolTip>
              </Form.Label>

              <Col sm="8">
                <Form.Control as="textarea"
                              placeholder={this.props.param.description}
                              rows={10}
                              onChange={(e) => {
                                  this.setJSONValue(e.currentTarget.value);
                              }}
                              value={this.props.value} />
                { this.state.error ?
                  <Alert variant="danger">
                    {this.state.error.code}
                  </Alert> :
                  <Button variant="default-outline"
                          className="full-width"
                          disabled={this.state.error}
                          onClick={() => this.setState({mode: "json"})}
                          size="sm">
                  <FontAwesomeIcon icon="pencil-alt"/>
                </Button>
                }
              </Col>
            </Form.Group>
        );
    }

    isEditable = (data, rowIdx)=>{
        let item = data[rowIdx];
        if (_.isString(item) || _.isUndefined(item) || _.isNull(item)){
            return true
        }
        return false;
    }

    displayValue = item=>{
        if (_.isString(item) || _.isUndefined(item) || _.isNull(item)){
            return item;
        }

        return serializeJSON(item);
    }

    renderTable = data=>{
        return (
            <Table className="paged-table csv-table">
              <thead>
                <tr>
                  <th className="metadata-control paged-table-header">
                    <ButtonGroup>
                      <Button
                        variant="default-outline" size="sm"
                        onClick={() => {
                            // Add an extra row at the current row index.
                            let data = JSONparse(this.props.value);
                            data.splice(0, 0, "");
                            this.props.setValue(serializeJSON(data));
                        }}
                      >
                        <FontAwesomeIcon icon="plus"/>
                      </Button>
                      <Button
                        variant="default-outline" size="sm"
                        onClick={()=>this.setState({mode: "text"})}
                      >
                        <FontAwesomeIcon icon="pencil-alt"/>
                      </Button>
                    </ButtonGroup>
                  </th>
                  <th>{T("Value")}</th>
                </tr>
              </thead>
              <tbody>
                {_.map(data, (value, rowIdx)=>{
                    return (
                        <tr key={rowIdx}>
                          <td className="metadata-control">
                            <ButtonGroup>
                              <Button
                                variant="default-outline" size="sm"
                                onClick={() => {
                                    // Add an extra row at
                                    // the current row index.
                                    let data = JSONparse(this.props.value);
                                    data.splice(rowIdx+1, 0, "");
                                    this.props.setValue(serializeJSON(data));
                                }}
                              >
                                <FontAwesomeIcon icon="plus"/>
                              </Button>
                              <Button
                                variant="default-outline" size="sm"
                                onClick={() => {
                                    // Drop the current row
                                    // at the current row
                                    // index.
                                    let data = JSONparse(this.props.value);
                                    data.splice(rowIdx, 1);
                                    this.props.setValue(serializeJSON(data));
                                }}
                              >
                                <FontAwesomeIcon icon="trash"/>
                              </Button>
                            </ButtonGroup>
                          </td>
                          <td>
                            <Form.Control
                              disabled={!this.isEditable(data, rowIdx)}
                              as="textarea" rows={1}
                              value={this.displayValue(value)}
                              onChange={e=>{
                                  data[rowIdx]=e.currentTarget.value;
                                  let new_data = serializeJSON(data);
                                  this.props.setValue(new_data);
                              }}
                            />
                          </td>
                        </tr>
                    );
                })}
              </tbody>
            </Table>
        );
    }

    renderJSONTable() {
        let param = this.props.param || {};
        let name = param.friendly_name || param.name;

        let data = JSONparse(this.props.value);
        return (
            <Form.Group as={Row}>
              <Form.Label column sm="3">
                <ToolTip tooltip={param.description}>
                  <div>
                    {name}
                  </div>
                </ToolTip>
              </Form.Label>

              <Col sm="8">
                {this.renderTable(data)}
              </Col>
            </Form.Group>
        );
    }
}
