import _ from 'lodash';
import React from 'react';
import PropTypes from 'prop-types';

import VeloLineChart from '../artifacts/line-charts.js';
import axios from 'axios';
import api from '../core/api-service.js';
import { PrepareData } from '../core/table.js';


export default class NotebookChart extends React.Component {
    static propTypes = {
        params: PropTypes.object,
    };

    state = {
        rows: [],
        columns: [],
        loading: true,
    }

    componentDidMount = () => {
        this.source = axios.CancelToken.source();
        this.fetchRows();
    }

    componentWillUnmount() {
        this.source.cancel();
    }

    fetchRows = () => {
        if (_.isEmpty(this.props.params)) {
            this.setState({loading: false});
            return;
        }

        let params = Object.assign({}, this.props.params);
        let url = "v1/GetTable";

        this.source.cancel();
        this.source = axios.CancelToken.source();

        api.get(url, params, this.source.token).then((response) => {
            if (response.cancel) {
                return;
            }

            let pageData = PrepareData(response.data);
            let columns = pageData.columns;
            this.setState({loading: false,
                           rows: pageData.rows,
                           columns: columns });
        }).catch(() => {
            this.setState({loading: false, rows: [], columns: []});
        });
    }

    render() {
        if (_.isEmpty(this.state.rows)) {
            return <></>;
        }
        return <VeloLineChart
                 className="col-12"
                 params={this.props.params}
                 columns={this.state.columns}
                 data={this.state.rows}
               />;
    }
};
