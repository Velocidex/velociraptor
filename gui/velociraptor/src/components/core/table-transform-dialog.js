import _ from 'lodash';

import React, { Component } from 'react';
import PropTypes from 'prop-types';
import Modal from 'react-bootstrap/Modal';
import Navbar from 'react-bootstrap/Navbar';
import ButtonGroup from 'react-bootstrap/ButtonGroup';
import Button from 'react-bootstrap/Button';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import VeloForm from '../forms/form.js';
import Form from 'react-bootstrap/Form';
import Row from 'react-bootstrap/Row';
import Col from 'react-bootstrap/Col';



export default class TableTransformDialog extends Component {
    static propTypes = {
        columns: PropTypes.array,
        transform: PropTypes.object,
        setTransform: PropTypes.func.isRequired,
        onClose: PropTypes.func.isRequired,
    };

    state = {
        sort_column: "Unset",
        sort_direction: "Ascending",
        filter_column: "Unset",
        filter_regex: "",
    }

    componentDidMount = () => {
        let transform = this.props.transform || {};
        this.setState({
            sort_column: transform.sort_column || "Unset",
            sort_direction: transform.sort_direction,
            filter_column: transform.filter_column || "Unset",
            filter_regex: transform.filter_regex,
        });
    }

    saveTransform = () => {
        let transform = {
            sort_column: this.state.sort_column,
            sort_direction: this.state.sort_direction,
            filter_column: this.state.filter_column,
            filter_regex: this.state.filter_regex,
        };
        if (transform.sort_column === "Unset") {
            transform.sort_column = undefined;
            transform.sort_direction = undefined;
        } else if (!transform.sort_direction) {
            transform.sort_direction = "Ascending";
        }

        if (transform.filter_column === "Unset") {
            transform.filter_column = undefined;
            transform.filter_regex = undefined;
        }

        this.props.setTransform(transform);
        this.props.onClose();
    }

    render() {
        let columns = ["Unset"];
        columns.push.apply(columns, this.props.columns);

        return (
            <Modal show={true}
                   className="full-height"
                   dialogClassName="modal-90w"
                   enforceFocus={false}
                   scrollable={true}
                   onHide={this.props.onClose}>
              <Modal.Header closeButton>
                <Modal.Title>Transform table
                </Modal.Title>
              </Modal.Header>
              <Modal.Body>
                <Form.Group as={Row}>
                  <Form.Label column sm="3">
                    Sort Column
                  </Form.Label>
                  <Col sm="8">
                    <ButtonGroup className="sort-button">
                      { this.state.sort_direction !== "Ascending" ?
                        <Button
                          disabled={this.state.sort_column === "Unset"}
                          onClick={()=>this.setState({sort_direction: "Ascending"})}
                          variant="outline-dark">
                          <FontAwesomeIcon icon="sort-alpha-down"/>
                        </Button>:
                        <Button
                          disabled={this.state.sort_column === "Unset"}
                          onClick={()=>this.setState({sort_direction: "Descending"})}
                          variant="outline-dark">
                          <FontAwesomeIcon icon="sort-alpha-up"/>
                        </Button>
                      }
                      <Form.Control as="select"
                                    value={this.state.sort_column}
                                    onChange={e=>this.setState({
                                        sort_column: e.currentTarget.value
                                    })}>
                        {_.map(columns || [], function(item, idx) {
                            return <option key={idx}>{item}</option>;
                        })}
                      </Form.Control>
                    </ButtonGroup>
                  </Col>
                </Form.Group>
                <VeloForm
                  param={{name: "filter_column",
                          friendly_name: "Filter Column",
                          type:"choices",
                          choices: columns}}
                  value={this.state.filter_column}
                  setValue={x=>this.setState({filter_column: x})}
                />
                { this.state.filter_column &&
                  this.state.filter_column !== "Unset" &&
                  <VeloForm
                    param={{name: "filter_regex",
                            friendly_name: "Filter Regex",
                            type:"regex",
                           }}
                    value={this.state.filter_regex}
                    setValue={x=>this.setState({filter_regex: x})}
                  />
                }
              </Modal.Body>
              <Modal.Footer>
                <Navbar className="w-100 justify-content-between">
                <ButtonGroup className="float-right">
                  <Button variant="default"
                          onClick={this.props.onClose}>
                    <FontAwesomeIcon icon="window-close"/>
                    <span className="button-label">Close</span>
                  </Button>
                  <Button variant="primary"
                          onClick={this.saveTransform}>
                    <FontAwesomeIcon icon="save"/>
                    <span className="button-label">Save</span>
                  </Button>
                </ButtonGroup>
                </Navbar>
              </Modal.Footer>
            </Modal>
        );
    }
}
