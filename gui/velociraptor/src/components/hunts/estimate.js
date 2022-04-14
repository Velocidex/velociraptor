import _ from 'lodash';

import React, { Component } from 'react';
import PropTypes from 'prop-types';
import Alert from 'react-bootstrap/Alert';
import Form from 'react-bootstrap/Form';
import api from '../core/api-service.js';
import axios from 'axios';
import Row from 'react-bootstrap/Row';
import Col from 'react-bootstrap/Col';
import './estimate.css';

export default class EstimateHunt extends Component {
    static propTypes = {
        params: PropTypes.object,
    }

    componentDidMount = () => {
        this.source = axios.CancelToken.source();
        this.fetchEstimate();
    }

    componentWillUnmount() {
        this.source.cancel("unmounted");
    }

    componentDidUpdate = (prevProps, prevState, rootNode) => {
        let hunt_request = this.makeHuntRequest(this.props.params);
        if (!_.isEqual(this.state.prev, hunt_request)) {
            this.fetchEstimate();
            return true;
        }
        return false;
    }

    state = {
        clients_affected: 0,

        // Default to calculating all clients because this is slightly
        // faster.
        last_active: "all",
        prev: {},
    }

    getLastActive = ()=>{
        switch(this.state.last_active) {
        case "1 Month":
            return 31 * 60 * 60 * 24;

        case "1 Week":
            return 7 * 60 * 60 * 24;

        case "1 Day":
            return 60 * 60 * 24;

        default:
            return 0;
        }
    }

    makeHuntRequest = (hunt_parameters)=>{
        let result = {
            last_active: this.getLastActive(),
            condition: {},
        };
        if (hunt_parameters.include_condition === "labels") {
            result.condition.labels = {"label": hunt_parameters.include_labels};
        }
        if (hunt_parameters.include_condition === "os" &&
            hunt_parameters.include_os !== "ALL") {
            result.condition.os = {"os": hunt_parameters.include_os};
        }

        if (hunt_parameters.exclude_condition === "labels") {
            result.condition.excluded_labels = {label: hunt_parameters.excluded_labels};
        }

        return result;
    }

    fetchEstimate = () => {
        let hunt_request = this.makeHuntRequest(this.props.params);
        this.setState({prev: hunt_request});

        api.post("v1/EstimateHunt",
                 hunt_request,
                 this.source.token).then(response=>{
                     this.setState({
                         clients_affected: response.data.total_clients_scheduled
                     });
                 });
    }

    render() {
        return (
            <>
              <Alert className="estimate" variant="info">
                <Form.Group as={Row}>
                  <Form.Label column sm="8">
                    Estimated affected clients { this.state.clients_affected || "0" }
                  </Form.Label>
                  <Col sm="3">
                    <Form.Control as="select"
                                  value={this.state.last_active}
                                  onChange={e=>this.setState({
                                      last_active: e.currentTarget.value
                                  })}
                    >
                      <option label="All known Clients" value="all">All Known Clients</option>
                      <option label="1 Day actives" value="1 Day">1 Day actives</option>
                      <option label="1 Week actives" value="1 Week">1 Week actives</option>
                      <option label="1 Month actives" value="1 Month">1 Month actives"</option>
                    </Form.Control>
                  </Col>
                </Form.Group>
              </Alert>
            </>
        );
    }
}
