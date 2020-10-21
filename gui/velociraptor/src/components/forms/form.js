import React from 'react';
import PropTypes from 'prop-types';

import _ from 'lodash';
import DateTimePicker from 'react-datetime-picker';
import Form from 'react-bootstrap/Form';
import Row from 'react-bootstrap/Row';
import Col from 'react-bootstrap/Col';
import Button from 'react-bootstrap/Button';
import ButtonGroup from 'react-bootstrap/ButtonGroup';

import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';

import BootstrapTable from 'react-bootstrap-table-next';
import cellEditFactory, { Type } from 'react-bootstrap-table2-editor';

import { parseCSV, serializeCSV } from '../utils/csv.js';

export default class VeloForm extends React.Component {
    static propTypes = {
        param: PropTypes.object,
        value: PropTypes.string,
        setValue: PropTypes.func.isRequired,
    };

    render() {
        let param = this.props.param;

        switch(param.type) {
        case "csv":
            let data = parseCSV(this.props.value);
            let columns = [{
                dataField: "_id",
                text: "",
                style: {
                    width: '8%',
                },
                formatter: (id, row) => {
                    return <ButtonGroup>
                             <Button variant="default-outline" size="sm"
                                     onClick={() => {
                                         // Add an extra row at the current row index.
                                         let data = parseCSV(this.props.value);
                                         data.data.splice(id, 0, {});
                                         this.props.setValue(
                                             serializeCSV(data.data,
                                                          data.columns));
                                     }}
                             >
                               <FontAwesomeIcon icon="plus"/>
                             </Button>
                             <Button variant="default-outline" size="sm"
                                     onClick={() => {
                                         // Drop th current row at the current row index.
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
                  <Form.Label column sm="3">{param.name}
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

        case "timestamp":
            let date = null;
            if (this.props.value) {
                try {
                    date = new Date(parseInt(this.props.value) * 1000);
                } catch(e) {};
            }

            return (
                <Form.Group as={Row}>
                  <Form.Label column sm="3">{param.name}</Form.Label>
                  <Col sm="8">
                    <DateTimePicker
                      onChange={(value) => {
                          if (!value) {
                              this.props.setValue(undefined);
                          } else {
                              this.props.setValue(parseInt(value.getTime() / 1000).toString());
                          }
                      }}
                      value={date}
                    />
                  </Col>
                </Form.Group>
            );

        case "choices":
            return (
                <Form.Group as={Row}>
                  <Form.Label column sm="3">{param.name}</Form.Label>
                  <Col sm="8">
                    <Form.Control as="select"
                                  onChange={(e) => {
                                      this.props.setValue(e.currentTarget.value);
                                  }}>
                      {_.map(this.props.param.choices || [], function(item, idx) {
                          return <option key={idx}>{item}</option>;
                      })}
                    </Form.Control>
                  </Col>
                </Form.Group>
            );

        case "bool":
            return (
                <Form.Group as={Row}>
                  <Form.Label column sm="3">{param.name}</Form.Label>
                  <Col sm="8">
                    <Form.Check
                      type="checkbox"
                      label={param.description}
                      onChange={(e) => {
                          if (e.currentTarget.checked) {
                              this.props.setValue("Y");
                          } else {
                              this.props.setValue(undefined);
                          }
                      }}
                      checked={this.props.value === "Y"}
                      value={this.props.value} />
                  </Col>
                </Form.Group>
            );

        default:
            return (
                <Form.Group as={Row}>
                  <Form.Label column sm="3">{param.name}</Form.Label>
                  <Col sm="8">
                    <Form.Control as="textarea"
                                  rows={1}
                                  onChange={(e) => {
                                      this.props.setValue(e.currentTarget.value);
                                  }}
                                  value={this.props.value} />
                  </Col>
                </Form.Group>
            );
        }
    };
}
