import _ from 'lodash';

import PropTypes from 'prop-types';
import { parseCSV, validateCSV, serializeCSV } from '../utils/csv.jsx';
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

export default class CSVForm extends Component {
    static propTypes = {
        param: PropTypes.object,
        value: PropTypes.string,
        setValue: PropTypes.func.isRequired,
    };

    state = {
        mode: "csv",
        error: "",
    }

    setCSValue = value=>{
        // Check if we can parse it properly.
        this.setState({error: validateCSV(value)});
        this.props.setValue(value);
    }

    render() {
        if (this.state.mode === "csv") {
            return this.renderCSVTable();
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
                                  this.setCSValue(e.currentTarget.value);
                              }}
                              value={this.props.value} />
                { this.state.error ?
                  <Alert variant="danger">
                    {this.state.error.code}
                  </Alert> :
                  <Button variant="default-outline"
                          className="full-width"
                          disabled={this.state.error}
                          onClick={() => this.setState({mode: "csv"})}
                          size="sm">
                  <FontAwesomeIcon icon="pencil-alt"/>
                </Button>
                }
              </Col>
            </Form.Group>
        );
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
                            let data = parseCSV(this.props.value);
                            data.data.splice(0, 0, {});
                            this.props.setValue(
                                serializeCSV(data.data,
                                             data.columns));
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
                  { _.map(data.columns, (c, idx)=>{
                      return <th key={idx}>{c}</th>;
                  })}
                </tr>
              </thead>
              <tbody>
                {_.map(data.data, (row, rowIdx)=>{
                    return (
                        <tr key={rowIdx}>
                          <td className="metadata-control">
                            <ButtonGroup>
                              <Button
                                variant="default-outline" size="sm"
                                onClick={() => {
                                    // Add an extra row at
                                    // the current row index.
                                    let data = parseCSV(this.props.value);
                                    data.data.splice(rowIdx+1, 0, {});
                                    this.props.setValue(
                                        serializeCSV(data.data,
                                                     data.columns));
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
                                    let data = parseCSV(this.props.value);
                                    data.data.splice(rowIdx, 1);
                                    this.props.setValue(
                                        serializeCSV(data.data,
                                                     data.columns));
                                }}
                              >
                                <FontAwesomeIcon icon="trash"/>
                              </Button>
                            </ButtonGroup>
                          </td>
                          {_.map(data.columns, (c, idx)=>{
                              return (
                                  <td key={idx}>
                                    <Form.Control
                                      as="textarea" rows={1}
                                      value={row[c] || ""}
                                      onChange={e=>{
                                          row[c]=e.currentTarget.value;
                                          let new_data = serializeCSV(
                                              data.data, data.columns);
                                          this.props.setValue(new_data);
                                      }}
                                    />
                                  </td>
                              );
                          })}
                        </tr>
                    );
                })}
              </tbody>
            </Table>
        );
    }

    renderCSVTable() {
        let param = this.props.param || {};
        let name = param.friendly_name || param.name;

        let data = parseCSV(this.props.value);
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
