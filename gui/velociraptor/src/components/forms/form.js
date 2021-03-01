import React from 'react';
import PropTypes from 'prop-types';
import moment from 'moment/moment.js';
import _ from 'lodash';
import DateTimePicker from 'react-datetime-picker';
import Form from 'react-bootstrap/Form';
import Row from 'react-bootstrap/Row';
import Col from 'react-bootstrap/Col';
import Button from 'react-bootstrap/Button';

import Tooltip from 'react-bootstrap/Tooltip';
import OverlayTrigger from 'react-bootstrap/OverlayTrigger';
import ButtonGroup from 'react-bootstrap/ButtonGroup';

import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';

import BootstrapTable from 'react-bootstrap-table-next';
import cellEditFactory, { Type } from 'react-bootstrap-table2-editor';

import { parseCSV, serializeCSV } from '../utils/csv.js';
const numberRegex = RegExp("^[0-9]+$");


// Returns a date object in local timestamp which represents the UTC
// date. This is needed because the date selector widget expects to
// work in local time.
function localTimeFromUTCTime(date) {
    return new Date(date.getUTCFullYear(), date.getUTCMonth(), date.getUTCDate(),
                    date.getUTCHours(), date.getUTCMinutes(), date.getUTCSeconds());
}

function convertToDate(x) {
    // Allow the value to be specified in a number of ways.
    if (_.isNumber(x) || numberRegex.test(x)) {
        try {
            return new Date(parseInt(x) * 1000);
        } catch(e) {};
    }

    try {
        let res = Date.parse(x);
        if (!_.isNaN(res)) {
            return new Date(res);
        }
    } catch (e) {};

    return null;
}

const renderToolTip = (props, params) => (
    <Tooltip show={params.description} {...props}>
       {params.description}
     </Tooltip>
);

export default class VeloForm extends React.Component {
    static propTypes = {
        param: PropTypes.object,
        value: PropTypes.string,
        setValue: PropTypes.func.isRequired,
    };

    state = {
        isUTC: true,

        // A Date() object that is parsed from value in local time.
        timestamp: null,
    }


    render() {
        let param = this.props.param;

        switch(param.type) {
        case "hidden":
            return <></>;
        case "csv":
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
                  <Form.Label column sm="3">
                    <OverlayTrigger
                      delay={{show: 250, hide: 400}}
                      overlay={(props)=>renderToolTip(props, param)}>
                      <div>
                        {param.name}
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

        case "timestamp":
            // value prop is always a string in ISO format in UTC timezone.
            let date = convertToDate(this.props.value);

            // Internally the date selector always uses local browser
            // time. If the form is configured to use utc mode, then
            // we convert the UTC time to the equivalent time in local
            // just for the form.
            if(_.isDate(date) && this.state.isUTC) {
                date = localTimeFromUTCTime(date);
            }
            return (
                <Form.Group as={Row}>
                  <Form.Label column sm="3">
                    <OverlayTrigger
                      delay={{show: 250, hide: 400}}
                      overlay={(props)=>renderToolTip(props, param)}>
                      <div>
                        {param.name}
                      </div>
                    </OverlayTrigger>
                  </Form.Label>
                  <Col sm="8">
                    <DateTimePicker
                      showLeadingZeros={true}
                      onChange={(value) => {
                          // Clear the prop value
                          if (!_.isDate(value)) {
                              this.props.setValue(undefined);

                              // If the form is in UTC we take the
                              // date the form gives us (which is in
                              // local timezone) and force the same
                              // date into a serialized ISO in Z time.
                          } else if(this.state.isUTC) {
                              let when = moment(value);
                              this.props.setValue(when.format('YYYY-MM-DDTHH:mm:ss') + 'Z');

                          } else {
                              // When in local time we just set the
                              // time as it is.
                              let when = moment(value);
                              this.props.setValue(when.toISOString());
                          }
                      }}
                      value={date}
                    />
                    {this.state.isUTC ?
                     <Button variant="default-outline"
                             onClick={() => this.setState({isUTC: false})}
                             size="sm">UTC</Button>:
                     <Button variant="default-outline"
                             onClick={() => this.setState({isUTC: true})}
                             size="sm">Local</Button>
                    }
                  </Col>
                </Form.Group>
            );

        case "choices":
            return (
                <Form.Group as={Row}>
                  <Form.Label column sm="3">
                    <OverlayTrigger
                      delay={{show: 250, hide: 400}}
                      overlay={(props)=>renderToolTip(props, param)}>
                      <div>
                        {param.name}
                      </div>
                    </OverlayTrigger>
                  </Form.Label>
                  <Col sm="8">
                    <Form.Control as="select"
                                  value={this.props.value}
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
                  <Form.Label column sm="3">
                    <OverlayTrigger
                      delay={{show: 250, hide: 400}}
                      overlay={(props)=>renderToolTip(props, param)}>
                      <div>
                        {param.name}
                      </div>
                    </OverlayTrigger>
                  </Form.Label>
                  <Col sm="8">
                    <Form.Check
                      type="checkbox"
                      label={param.description}
                      onChange={(e) => {
                          if (e.currentTarget.checked) {
                              this.props.setValue("Y");
                          } else {
                              this.props.setValue("N");
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
                  <Form.Label column sm="3">
                    <OverlayTrigger
                      delay={{show: 250, hide: 400}}
                      overlay={(props)=>renderToolTip(props, param)}>
                      <div>
                        {param.name}
                      </div>
                    </OverlayTrigger>
                  </Form.Label>
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
