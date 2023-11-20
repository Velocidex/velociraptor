import _ from 'lodash';

import PropTypes from 'prop-types';
import { parseCSV, validateCSV, serializeCSV } from '../utils/csv.jsx';
import ButtonGroup from 'react-bootstrap/ButtonGroup';
import Button from 'react-bootstrap/Button';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import Form from 'react-bootstrap/Form';
import cellEditFactory, { Type } from 'react-bootstrap-table2-editor';
import Row from 'react-bootstrap/Row';
import React, { Component } from 'react';
import OverlayTrigger from 'react-bootstrap/OverlayTrigger';
import Tooltip from 'react-bootstrap/Tooltip';
import Col from 'react-bootstrap/Col';
import BootstrapTable from 'react-bootstrap-table-next';
import Alert from 'react-bootstrap/Alert';


const renderToolTip = (props, params) => (
    <Tooltip show={params.description} {...props}>
       {params.description}
     </Tooltip>
);

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
                <OverlayTrigger
                  delay={{show: 250, hide: 400}}
                  overlay={(props)=>renderToolTip(props, param)}>
                  <div>
                    {name}
                  </div>
                </OverlayTrigger>
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

    renderCSVTable() {
        let param = this.props.param || {};
        let name = param.friendly_name || param.name;

        let data = parseCSV(this.props.value);
        let columns = [{
            dataField: "_id",
            text: "",
            style: {
                width: '8%',
            },
            headerFormatter: (column, colIndex) => {
                if (colIndex === 0) {
                    return <ButtonGroup>
                             <Button variant="default-outline" size="sm"
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
                             <Button variant="default-outline" size="sm"
                                     onClick={()=>this.setState({mode: "text"})}
                             >
                               <FontAwesomeIcon icon="pencil-alt"/>
                             </Button>
                           </ButtonGroup>;
                };
                return column;
            },
            formatter: (id, row) => {
                return <ButtonGroup>
                         <Button variant="default-outline" size="sm"
                                 onClick={() => {
                                     // Add an extra row at the current row index.
                                     let data = parseCSV(this.props.value);
                                     data.data.splice(id+1, 0, {});
                                     this.props.setValue(
                                         serializeCSV(data.data,
                                                      data.columns));
                                 }}
                         >
                           <FontAwesomeIcon icon="plus"/>
            </Button>
            <Button variant="default-outline" size="sm"
                    onClick={() => {
                        // Drop the current row at the current row index.
                        let data = parseCSV(this.props.value);
                        data.data.splice(id, 1);
                        this.props.setValue(
                            serializeCSV(data.data,
                                         data.columns));
                    }}
            >
              <FontAwesomeIcon icon="trash"/>
            </Button>
          </ButtonGroup>;
            },
        }];
        _.each(data.columns, (name) => {
            columns.push({dataField: name,
                          editor: {
                              type: Type.TEXTAREA
                          },
                          text: name});
        });

        _.map(data.data, (item, idx) => {item["_id"] = idx;});

        return (
            <Form.Group as={Row}>
              <Form.Label column sm="3">
                <OverlayTrigger
                  delay={{show: 250, hide: 400}}
                  overlay={(props)=>renderToolTip(props, param)}>
                  <div>
                    {name}
                  </div>
                </OverlayTrigger>
              </Form.Label>

              <Col sm="8">
                <BootstrapTable
                  hover condensed bootstrap4
                  data={data.data}
                  keyField="_id"
                  columns={columns}
                  cellEdit={ cellEditFactory({
                      mode: 'click',
                      afterSaveCell: (oldValue, newValue, row, column) => {
                          // Update the CSV value.
                          let new_data = serializeCSV(data.data, data.columns);
                          this.props.setValue(new_data);
                      },
                      blurToSave: true,
                  }) }
                />
              </Col>
            </Form.Group>
        );
    }
}
