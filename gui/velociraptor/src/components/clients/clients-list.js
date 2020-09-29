import "./clients-list.css";

import React, { Component } from 'react';
import PropTypes from 'prop-types';
import { withRouter }  from "react-router-dom";

import api from '../core/api-service.js';
import VeloClientStatusIcon from "./client-status.js";


class VeloClientList extends Component {
    static propTypes = {
        query: PropTypes.string.isRequired,
        setClient: PropTypes.func.isRequired,
    }

    state = {
        clients: [],
        idx2labels: {},
        labels2idex: {},
    };

    constructor(props) {
        super(props);

        this.state = {
            clients: [],
            idx2labels: {},
            labels2idex: {},
        };

        this.searchClients = this.searchClients.bind(this);
        this.addToLabels = this.addToLabels.bind(this);
        this.openClientInfo = this.openClientInfo.bind(this);
    }

    componentDidMount() {
        this.searchClients();
    }

    componentDidUpdate(prevProps) {
        if (this.props.query !== prevProps.query) {
            this.searchClients();
        }
    }

    addToLabels(index, client_id)  {
        /*
          this.state.idx2labels[index] = client_id;
          this.state.labels2idex[client_id] = index;
        */
    }

    searchClients() {
        var query = this.props.query;
        if (!query) {
            return;
        }

        api.get('/api/v1/SearchClients', {
            query: query,
            count: 50,

        }).then(resp => {
            if (resp.data && resp.data.items) {
                this.setState({
                    isLoading: false,
                    clients: resp.data.items,
                    idx2labels: {},
                    labels2idex: {},
                });
            }
            return true;
        });
    }

    openClientInfo(e, client) {
        e.preventDefault();
        this.props.setClient(client);

        // Navigate to the host_info page.
        this.props.history.push('/host/' + client.client_id);
    }

    renderTableRows(rows) {
        return rows.map((item, index) => {
            return (
                <tr ng-click="controller.onClientClick(item)"
                    key={item.client_id}
                    onClick={(e)=>this.openClientInfo(e, item)}
                >
                  <td>
                    <input type="checkbox" className="client-checkbox"
                           onChange={() => this.addToLabels(item.client_id, index)}
                    ></input>
                  </td>
                  <td><VeloClientStatusIcon client={item}/></td>
                  <td>
                    <span type="subject">{item.client_id}</span>
                  </td>

                  <td>
                    {item.os_info.fqdn}
                  </td>

                  <td>
                    {item.os_info.release}
                  </td>

                  <td>
                    {item.labels}
                  </td>
                </tr>
            );
        });
    }

    render() {
        return (
            <>
              Search for { this.props.query }
              <div className="search-table">
                <table className="table table-striped table-condensed table-hover table-bordered full-width">
                  <colgroup>
                    <col style={{width: "40px"}} />
                    <col style={{width: "40px"}} />
                    <col style={{width: "13em"}} />
                    <col style={{width: "13em"}} />
                    <col style={{width: "15%"}} />
                  </colgroup>
                  <thead>
                    <tr>
                      <th><input type="checkbox" className="client-checkbox select-all"
                    ng-model="controller.allClientsSelected"
                    ng-change="controller.selectAll()"></input>
                      </th>
                      <th>Online</th>
                      <th>ClientID</th>
                      <th>Host</th>
                      <th>OS Version</th>
                      <th>Labels</th>
                    </tr>
                  </thead>
                  <tbody>
                    { this.renderTableRows(this.state.clients) }
                  </tbody>
                </table>
              </div>
            </>
        );
    }
};


export default withRouter(VeloClientList);
